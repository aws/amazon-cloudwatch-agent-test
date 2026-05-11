//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package efa

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
		gt.nodes[n.Name] = n
	}
	for _, p := range podList.Items {
		gt.pods[p.Namespace+"/"+p.Name] = p
	}
	return gt, nil
}

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

// imageTagFromPod returns the image tag of the first container of the first
// pod matching the given label selector in the given namespace.
// Returns empty string (and logs via t.Logf) if no matching pod is found.
func imageTagFromPod(t *testing.T, gt *k8sGroundTruth, namespace, labelKey, labelValue string) string {
	t.Helper()
	for _, p := range gt.pods {
		p := p
		if p.Namespace != namespace {
			continue
		}
		if v, ok := p.Labels[labelKey]; ok && v == labelValue {
			if len(p.Spec.Containers) == 0 {
				continue
			}
			image := p.Spec.Containers[0].Image
			if idx := strings.LastIndex(image, ":"); idx != -1 {
				return image[idx+1:]
			}
			return ""
		}
	}
	t.Logf("no pod found with %s=%s in namespace %s", labelKey, labelValue, namespace)
	return ""
}
