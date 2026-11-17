//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Azure attribute tests check the AKS counterparts of the EKS host identity (host.type, host.id) and
// the azure.aks resource attributes, with the same expected values as aksResourceExpectations in
// test/azure/aks. They have no EKS equivalent file.

package gpudriver

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDCGMAKSResourceAttributes validates the azure.aks resource attributes on DCGM metrics.
func TestDCGMAKSResourceAttributes(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				res := r.Labels.Resource
				require.Equal(t, "azure", res["cloud.provider"], "%s cloud.provider", metricName)
				require.Equal(t, "azure.aks", res["cloud.platform"], "%s cloud.platform", metricName)
				require.Equal(t, env.AKSClusterName, res["k8s.cluster.name"], "%s k8s.cluster.name", metricName)
				require.Equal(t, env.AzureLocation, res["cloud.region"], "%s cloud.region", metricName)
				require.Equal(t, env.AzureResourceGroup, res["azure.resourcegroup.name"], "%s azure.resourcegroup.name", metricName)
				require.NotEmpty(t, res["cloud.account.id"], "%s cloud.account.id", metricName)
			}
		})
	}
}

// TestDCGMHostAttributes validates the GPU host identity on each node pool, the Azure counterpart of the
// EKS host.type (instance type) and host.id (i-…) checks: azure.vm.size matches the node's instance-type
// label, and the VM, scale set and host name belong to the pool's node.
func TestDCGMHostAttributes(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")
	for _, ng := range gpuPools {
		ng := ng
		t.Run(ng.Description+"/"+ng.Pool, func(t *testing.T) {
			nodes := poolNodes(gt, ng.Pool)
			require.Len(t, nodes, 1, "expected one node in pool %s", ng.Pool)
			wantSize := nodes[0].Labels["node.kubernetes.io/instance-type"]
			require.NotEmpty(t, wantSize, "%s node has no node.kubernetes.io/instance-type label", ng.Pool)

			poolResults := filterByPool(results, ng.Pool)
			require.NotEmpty(t, poolResults, "No DCGM_FI_DEV_GPU_UTIL results from %s node", ng.Pool)
			for _, r := range poolResults {
				res := r.Labels.Resource
				require.Equal(t, wantSize, res["azure.vm.size"], "%s azure.vm.size", ng.Pool)
				require.NotEmpty(t, res["host.id"], "%s host.id", ng.Pool)
				require.Equal(t, res["k8s.node.name"], res["host.name"], "%s host.name should equal k8s.node.name", ng.Pool)
				require.True(t, strings.HasPrefix(res["azure.vm.name"], "aks-"+ng.Pool+"-"),
					"%s azure.vm.name should start with aks-%s-, got %q", ng.Pool, ng.Pool, res["azure.vm.name"])
				require.True(t, strings.HasPrefix(res["azure.vm.scaleset.name"], "aks-"+ng.Pool+"-"),
					"%s azure.vm.scaleset.name should start with aks-%s-, got %q", ng.Pool, ng.Pool, res["azure.vm.scaleset.name"])
			}
		})
	}
}

// TestDCGMScrapePath validates the metrics come through the chart's dcgm-exporter scrape job (the
// exporter pod on :9400), not the AKS host DCGM job used on managed GPU pools.
func TestDCGMScrapePath(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				require.Equal(t, "dcgm-exporter", r.Labels.Resource["service.name"], "%s service.name", metricName)
			}
		})
	}
}
