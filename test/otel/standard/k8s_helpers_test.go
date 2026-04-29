//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
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
		gt.nodes[n.Name] = n
	}
	for _, p := range podList.Items {
		gt.pods[p.Namespace+"/"+p.Name] = p
	}
	return gt, nil
}

// imageTagFromPod finds a pod by namespace+label and returns the image tag of its first container.
func imageTagFromPod(t *testing.T, gt *k8sGroundTruth, namespace, labelKey, labelValue string) string {
	t.Helper()
	for _, p := range gt.pods {
		if p.Namespace != namespace {
			continue
		}
		if v, ok := p.Labels[labelKey]; ok && v == labelValue {
			if len(p.Spec.Containers) > 0 {
				image := p.Spec.Containers[0].Image
				if idx := strings.LastIndex(image, ":"); idx != -1 {
					return image[idx+1:]
				}
			}
		}
	}
	t.Logf("WARNING: no pod found with %s=%s in %s", labelKey, labelValue, namespace)
	return ""
}

// k8sServerVersion returns the Kubernetes API server version string.
func k8sServerVersion(t *testing.T) string {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("building kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("creating K8s clientset: %v", err)
	}
	sv, err := clientset.Discovery().ServerVersion()
	if err != nil {
		t.Fatalf("getting K8s server version: %v", err)
	}
	return sv.GitVersion
}

// lookupPod finds a pod by name and optional namespace. Uses the
// namespace/name key for O(1) lookup. Otherwise falls back to linear scan.
func (gt *k8sGroundTruth) lookupPod(podName, namespace string) (corev1.Pod, bool) {
	if namespace != "" {
		p, ok := gt.pods[namespace+"/"+podName]
		return p, ok
	}
	for _, p := range gt.pods {
		if p.Name == podName {
			return p, true
		}
	}
	return corev1.Pod{}, false
}

// parseInstanceIDFromProviderID extracts the EC2 instance ID from a
// Kubernetes node's spec.providerID.
// Format: "aws:///us-east-1a/i-0abc123def456" → "i-0abc123def456"
func parseInstanceIDFromProviderID(providerID string) (string, error) {
	parts := strings.Split(providerID, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("empty provider ID")
	}
	instanceID := parts[len(parts)-1]
	if !strings.HasPrefix(instanceID, "i-") {
		return "", fmt.Errorf("provider ID %q: last segment %q does not start with 'i-'", providerID, instanceID)
	}
	return instanceID, nil
}
