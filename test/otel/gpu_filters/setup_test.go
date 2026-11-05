//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu_filters

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	keepNamespace = "gpu-keep"
	dropNamespace = "gpu-drop"

	multiGPUNodeLabel = "ci-test.example.com/multi-gpu"
	gpuResource       = corev1.ResourceName("nvidia.com/gpu")
	// expectedGPUCount is the number of GPUs on the multi-GPU node. The two
	// gpu-burn pods use one each, so two GPUs stay unused.
	expectedGPUCount = 4
)

var (
	cfg    otelmetrics.TestConfig
	client *otelmetrics.OtelMetricsClient
	// keepPod and dropPod are the names of the gpu-burn pods in gpu-keep and
	// gpu-drop, and dropPodUID is the UID of the gpu-drop pod. node is the
	// multi-GPU node that runs both.
	keepPod, dropPod, dropPodUID, node string
)

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env := environment.GetEnvironmentMetaData()

	region := env.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		fmt.Fprintf(os.Stderr, "Region not set\n")
		os.Exit(1)
	}

	clusterName := env.EKSClusterName
	if clusterName == "" {
		clusterName = os.Getenv("CLUSTER_NAME")
	}
	if clusterName == "" {
		fmt.Fprintf(os.Stderr, "Cluster name not set\n")
		os.Exit(1)
	}

	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		fmt.Fprintf(os.Stderr, "AWS config error: %v\n", err)
		os.Exit(1)
	}
	stsClient := sts.NewFromConfig(awsCfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "STS GetCallerIdentity error: %v\n", err)
		os.Exit(1)
	}

	cfg = otelmetrics.TestConfig{
		Region:         region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		ClusterName:    clusterName,
		AccountID:      *identity.Account,
		SigningService: "monitoring",
	}

	client, err = otelmetrics.NewClient(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client error: %v\n", err)
		os.Exit(1)
	}

	if err := findPods(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Kubernetes error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// findPods sets keepPod, dropPod, dropPodUID and node through the kubeconfig that
// Terraform writes for kubectl. It fails unless both gpu-burn pods run on one
// multi-GPU node with exactly expectedGPUCount GPUs.
func findPods(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	keep, err := runningGPUBurnPod(ctx, clientset, keepNamespace)
	if err != nil {
		return err
	}
	drop, err := runningGPUBurnPod(ctx, clientset, dropNamespace)
	if err != nil {
		return err
	}
	if keep.Spec.NodeName != drop.Spec.NodeName {
		return fmt.Errorf("gpu-burn pods run on different nodes: %s on %s, %s on %s",
			keep.Name, keep.Spec.NodeName, drop.Name, drop.Spec.NodeName)
	}

	n, err := clientset.CoreV1().Nodes().Get(ctx, keep.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if n.Labels[multiGPUNodeLabel] != "true" {
		return fmt.Errorf("node %s has no label %s=true", n.Name, multiGPUNodeLabel)
	}
	gpus := n.Status.Allocatable[gpuResource]
	if gpus.Value() != expectedGPUCount {
		return fmt.Errorf("node %s has %s GPUs, want %d", n.Name, gpus.String(), expectedGPUCount)
	}

	keepPod, dropPod, dropPodUID, node = keep.Name, drop.Name, string(drop.UID), n.Name
	return nil
}

// runningGPUBurnPod returns the one gpu-burn pod of namespace, which must be
// running.
func runningGPUBurnPod(ctx context.Context, clientset kubernetes.Interface, namespace string) (corev1.Pod, error) {
	list, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=gpu-burn"})
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(list.Items) != 1 {
		return corev1.Pod{}, fmt.Errorf("want 1 gpu-burn pod in %s, got %d", namespace, len(list.Items))
	}
	pod := list.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return corev1.Pod{}, fmt.Errorf("gpu-burn pod %s/%s is %s, want Running", namespace, pod.Name, pod.Status.Phase)
	}
	return pod, nil
}
