//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package multi_efa_filters

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	keepNamespace = "efa-keep"
	dropNamespace = "efa-drop"

	multiEfaNodeLabel = "ci-test.example.com/multi-efa-sm"
	freeNodeLabel     = "ci-test.example.com/multi-efa-free"
	efaResource       = corev1.ResourceName("vpc.amazonaws.com/efa")
	// expectedEFACount is the number of EFA devices on each multi-EFA node.
	// On node 1, one efaburn pod uses each card, so no card is free. On
	// node 2, the efaburn-free pod uses one card, and the other is free.
	expectedEFACount = 2

	agentNamespace = "amazon-cloudwatch"
	agentDaemonSet = "cloudwatch-agent"
)

var (
	cfg    otelmetrics.TestConfig
	client *otelmetrics.OtelMetricsClient
	// keepPod and dropPod are the names of the efaburn pods in efa-keep and
	// efa-drop, and dropPodUID is the UID of the efa-drop pod. node is the
	// multi-EFA node (node 1) that runs both.
	keepPod, dropPod, dropPodUID, node string
	// freePod and freePodUID are the name and UID of the efaburn-free pod in
	// efa-drop. freeNode is the multi-EFA node (node 2) that runs it.
	freePod, freePodUID, freeNode string
	// clientset reads the agent pods, see agentStart.
	clientset kubernetes.Interface
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

// findPods sets keepPod, dropPod, dropPodUID, node, freePod, freePodUID and
// freeNode through the kubeconfig that Terraform writes for kubectl. It fails
// unless node 1 runs exactly the efa-keep and efa-drop efaburn pods, node 2
// runs exactly the efaburn-free pod, and each node has expectedEFACount EFA
// devices.
func findPods(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	clientset = cs

	keep, err := runningPod(ctx, keepNamespace, "app=efaburn")
	if err != nil {
		return err
	}
	drop, err := runningPod(ctx, dropNamespace, "app=efaburn")
	if err != nil {
		return err
	}
	free, err := runningPod(ctx, dropNamespace, "app=efaburn-free")
	if err != nil {
		return err
	}
	if keep.Spec.NodeName != drop.Spec.NodeName {
		return fmt.Errorf("efaburn pods run on different nodes: %s on %s, %s on %s",
			keep.Name, keep.Spec.NodeName, drop.Name, drop.Spec.NodeName)
	}
	if free.Spec.NodeName == keep.Spec.NodeName {
		return fmt.Errorf("efaburn-free pod %s runs on node 1 %s, want another node", free.Name, free.Spec.NodeName)
	}

	if err := checkNode(ctx, keep.Spec.NodeName, multiEfaNodeLabel, keep.Name, drop.Name); err != nil {
		return err
	}
	if err := checkNode(ctx, free.Spec.NodeName, freeNodeLabel, free.Name); err != nil {
		return err
	}

	keepPod, dropPod, dropPodUID, node = keep.Name, drop.Name, string(drop.UID), keep.Spec.NodeName
	freePod, freePodUID, freeNode = free.Name, string(free.UID), free.Spec.NodeName
	return nil
}

// checkNode fails unless the node has label=true and expectedEFACount EFA
// devices, and its pods in efa-keep and efa-drop are exactly pods.
func checkNode(ctx context.Context, name, label string, pods ...string) error {
	n, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if n.Labels[label] != "true" {
		return fmt.Errorf("node %s has no label %s=true", n.Name, label)
	}
	efa := n.Status.Allocatable[efaResource]
	if efa.Value() != expectedEFACount {
		return fmt.Errorf("node %s has %s EFA devices, want %d", n.Name, efa.String(), expectedEFACount)
	}

	var got []string
	for _, ns := range []string{keepNamespace, dropNamespace} {
		list, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
		if err != nil {
			return err
		}
		for _, p := range list.Items {
			got = append(got, p.Name)
		}
	}
	sort.Strings(got)
	want := append([]string(nil), pods...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		return fmt.Errorf("node %s runs the test pods %q, want %q", name, got, want)
	}
	return nil
}

// runningPod returns the one pod of namespace with the label selector, which
// must be running.
func runningPod(ctx context.Context, namespace, selector string) (corev1.Pod, error) {
	list, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(list.Items) != 1 {
		return corev1.Pod{}, fmt.Errorf("want 1 pod %s in %s, got %d", selector, namespace, len(list.Items))
	}
	pod := list.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return corev1.Pod{}, fmt.Errorf("pod %s/%s is %s, want Running", namespace, pod.Name, pod.Status.Phase)
	}
	return pod, nil
}

// agentStart returns the latest container start of the cloudwatch-agent
// DaemonSet pod on the node.
func agentStart(ctx context.Context, nodeName string) (time.Time, error) {
	list, err := clientset.CoreV1().Pods(agentNamespace).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + nodeName})
	if err != nil {
		return time.Time{}, err
	}
	for _, p := range list.Items {
		owned := false
		for _, o := range p.OwnerReferences {
			if o.Kind == "DaemonSet" && o.Name == agentDaemonSet {
				owned = true
			}
		}
		if !owned {
			continue
		}
		var start time.Time
		for _, c := range p.Status.ContainerStatuses {
			if c.State.Running == nil {
				return time.Time{}, fmt.Errorf("agent pod %s container %s is not running", p.Name, c.Name)
			}
			if c.State.Running.StartedAt.Time.After(start) {
				start = c.State.Running.StartedAt.Time
			}
		}
		if start.IsZero() {
			return time.Time{}, fmt.Errorf("agent pod %s has no running container", p.Name)
		}
		return start, nil
	}
	return time.Time{}, fmt.Errorf("no %s/%s pod on node %s", agentNamespace, agentDaemonSet, nodeName)
}
