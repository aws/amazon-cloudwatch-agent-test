//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Path tests validate the managed-GPU path: AKS marks the node for host DCGM, the chart's
// dcgm-exporter pod is kept off it (dcgmExporter.affinityAKS), so every GPU is reported exactly once.

package gpu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	agentNamespace     = "amazon-cloudwatch"
	dcgmExporterLabel  = "app.kubernetes.io/component"
	dcgmExporterValue  = "dcgm-exporter"
	managedDCGMLabel   = "kubernetes.azure.com/dcgm-exporter"
	agentPoolLabel     = "kubernetes.azure.com/agentpool"
	managedDCGMEnabled = "enabled"
)

type k8sGroundTruth struct {
	nodes []corev1.Node
	pods  []corev1.Pod
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
	require.NoError(t, groundTruthErr, "building K8s ground truth")
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

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	return &k8sGroundTruth{nodes: nodes.Items, pods: pods.Items}, nil
}

// poolNode returns the single node of an AKS node pool.
func (gt *k8sGroundTruth) poolNode(t *testing.T, pool string) corev1.Node {
	t.Helper()
	var found []corev1.Node
	for _, n := range gt.nodes {
		if n.Labels[agentPoolLabel] == pool {
			found = append(found, n)
		}
	}
	require.Len(t, found, 1, "expected one node in pool %s", pool)
	return found[0]
}

// dcgmExporterPodsOn returns the chart's dcgm-exporter pods scheduled on the node.
func (gt *k8sGroundTruth) dcgmExporterPodsOn(nodeName string) []corev1.Pod {
	var out []corev1.Pod
	for _, p := range gt.pods {
		if p.Namespace == agentNamespace && p.Labels[dcgmExporterLabel] == dcgmExporterValue && p.Spec.NodeName == nodeName {
			out = append(out, p)
		}
	}
	return out
}

// TestDCGMNodePoolLabels validates AKS marks the managed-GPU node for host DCGM, the label the
// chart's dcgm-host job keeps and dcgmExporter.affinityAKS excludes.
func TestDCGMNodePoolLabels(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)
	managed := gt.poolNode(t, managedPool)
	require.Equal(t, managedDCGMEnabled, managed.Labels[managedDCGMLabel], "%s node %s label", managedPool, managedDCGMLabel)
}

// TestDCGMExporterPodPlacement validates the chart's exporter pod is not scheduled on the managed-GPU node.
func TestDCGMExporterPodPlacement(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)
	managed := gt.poolNode(t, managedPool)
	require.Empty(t, gt.dcgmExporterPodsOn(managed.Name), "chart dcgm-exporter pod scheduled on managed-GPU node %s", managed.Name)
}

// TestDCGMEachGPUReportedOnce validates every GPU UUID is reported by exactly one node, one GPU on the managed pool.
func TestDCGMEachGPUReportedOnce(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	uuidNodes := make(map[string]map[string]struct{})
	for _, r := range results {
		uuid := r.Labels.Datapoint["UUID"]
		node := r.Labels.Resource["k8s.node.name"]
		require.NotEmpty(t, uuid, "result without UUID on node %q", node)
		if uuidNodes[uuid] == nil {
			uuidNodes[uuid] = make(map[string]struct{})
		}
		uuidNodes[uuid][node] = struct{}{}
	}
	for uuid, nodes := range uuidNodes {
		require.Len(t, nodes, 1, "GPU %s reported by more than one node: %v", uuid, nodes)
	}
	uuids := make(map[string]struct{})
	for _, r := range onPool(results, managedPool) {
		uuids[r.Labels.Datapoint["UUID"]] = struct{}{}
	}
	require.Len(t, uuids, 1, "expected one GPU on %s, got %d", managedPool, len(uuids))
}
