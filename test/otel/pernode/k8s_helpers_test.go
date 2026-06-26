//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// agentNamespace is where the operator, agent and Target Allocator run.
	agentNamespace = "amazon-cloudwatch"
	// targetAllocatorDeploymentName is the TA Deployment created by the operator.
	targetAllocatorDeploymentName = "cloudwatch-agent-target-allocator"
)

// k8sGroundTruth holds node and pod data fetched from the Kubernetes API.
type k8sGroundTruth struct {
	nodes map[string]corev1.Node // keyed by metadata.name
	pods  map[string]corev1.Pod  // keyed by "namespace/name"
}

var (
	groundTruth     *k8sGroundTruth
	groundTruthOnce sync.Once
	groundTruthErr  error
)

func kubeconfigPath() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}
	return filepath.Join(os.Getenv("HOME"), ".kube", "config")
}

// k8sClientset builds a Kubernetes clientset from the ambient kubeconfig.
func k8sClientset(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	if err != nil {
		t.Fatalf("building kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("creating K8s clientset: %v", err)
	}
	return clientset
}

// getGroundTruth returns the shared ground truth, initializing it on first call.
func getGroundTruth(t *testing.T) *k8sGroundTruth {
	t.Helper()
	groundTruthOnce.Do(func() {
		groundTruth, groundTruthErr = buildGroundTruth()
	})
	if groundTruthErr != nil {
		t.Fatalf("failed to build K8s ground truth: %v", groundTruthErr)
	}
	return groundTruth
}

func buildGroundTruth() (*k8sGroundTruth, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating K8s clientset: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	if len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("K8s API returned 0 nodes")
	}

	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	gt := &k8sGroundTruth{
		nodes: make(map[string]corev1.Node, len(nodeList.Items)),
		pods:  make(map[string]corev1.Pod, len(podList.Items)),
	}
	for _, n := range nodeList.Items {
		n := n
		gt.nodes[n.Name] = n
	}
	for _, p := range podList.Items {
		p := p
		gt.pods[p.Namespace+"/"+p.Name] = p
	}
	return gt, nil
}

// nodeNames returns the set of Kubernetes node names.
func (gt *k8sGroundTruth) nodeNames() map[string]struct{} {
	out := make(map[string]struct{}, len(gt.nodes))
	for name := range gt.nodes {
		out[name] = struct{}{}
	}
	return out
}

// crdServed reports whether the given groupVersion exposes the named resource,
// i.e. the CRD is installed and established. Uses discovery so it needs no
// apiextensions client dependency.
func crdServed(t *testing.T, clientset *kubernetes.Clientset, groupVersion, resource string) bool {
	t.Helper()
	list, err := clientset.Discovery().ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		// A missing group surfaces as a discovery error; treat as not served.
		t.Logf("discovery for %s returned: %v", groupVersion, err)
		return false
	}
	for _, r := range list.APIResources {
		if r.Name == resource {
			return true
		}
	}
	return false
}

// gvrServiceMonitor / gvrPodMonitor identify the community CRDs we expect bundled.
var (
	gvrServiceMonitor = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"}
	gvrPodMonitor     = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "podmonitors"}
)

// targetAllocatorDeployment fetches the Target Allocator Deployment.
func targetAllocatorDeployment(t *testing.T, clientset *kubernetes.Clientset) *appsv1.Deployment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(ctx, targetAllocatorDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting Target Allocator deployment %s/%s: %v", agentNamespace, targetAllocatorDeploymentName, err)
	}
	return dep
}

// targetAllocatorPods lists the Target Allocator pods.
func targetAllocatorPods(t *testing.T, clientset *kubernetes.Clientset) []corev1.Pod {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := clientset.CoreV1().Pods(agentNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + targetAllocatorDeploymentName,
	})
	if err != nil {
		t.Fatalf("listing Target Allocator pods: %v", err)
	}
	return list.Items
}

// totalRestarts sums container restart counts across the given pods.
func totalRestarts(pods []corev1.Pod) int32 {
	var total int32
	for _, p := range pods {
		for _, cs := range p.Status.ContainerStatuses {
			total += cs.RestartCount
		}
	}
	return total
}
