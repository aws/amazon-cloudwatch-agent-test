//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// k8sGroundTruth holds node and pod data fetched from the Kubernetes API, to compare with what the agent
// reported.
type k8sGroundTruth struct {
	nodes map[string]corev1.Node
	pods  map[string]corev1.Pod
}

var (
	groundTruth     *k8sGroundTruth
	groundTruthOnce sync.Once
	groundTruthErr  error
)

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

// buildGroundTruth reads KUBECONFIG, which the AKS Terraform stack exports to go test. The API server only
// accepts the runner's IP.
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
