//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const agentNamespace = "amazon-cloudwatch"

func liveAgentPods(ctx context.Context) (map[string]struct{}, error) {
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
		return nil, fmt.Errorf("creating clientset: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	podList, err := clientset.CoreV1().Pods(agentNamespace).List(cctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	live := make(map[string]struct{}, len(podList.Items))
	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil {
			live[p.Name] = struct{}{}
		}
	}
	return live, nil
}

func filterToLivePods(t *testing.T, results []otelmetrics.RangeResult, live map[string]struct{}, metricName string) []otelmetrics.RangeResult {
	t.Helper()
	var filtered []otelmetrics.RangeResult
	for _, series := range results {
		podName := series.Labels.Resource["k8s.pod.name"]
		if _, ok := live[podName]; !ok {
			t.Logf("  dropping %s from %s — pod is not currently running (stale/terminated)", podName, metricName)
			continue
		}
		filtered = append(filtered, series)
	}
	return filtered
}
