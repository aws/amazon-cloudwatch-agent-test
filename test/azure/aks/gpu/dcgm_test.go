//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestDCGMInstrumentationScope validates the scope name and CloudWatch scope attributes the
// dcgm_host pipeline stamps. The version is only checked for presence (the chart sets the exporter
// image tag, not the AKS host DCGM version).
func TestDCGMInstrumentationScope(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			require.Equal(t, "github.com/NVIDIA/dcgm-exporter", r.Labels.Instrumentation["@name"], "%s on %s: @instrumentation.@name", metricName, pool)
			require.NotEmpty(t, r.Labels.Instrumentation["@version"], "%s on %s: @instrumentation.@version", metricName, pool)
			require.Equal(t, "cloudwatch-agent", r.Labels.Instrumentation["cloudwatch.source"], "%s on %s: cloudwatch.source", metricName, pool)
			require.Equal(t, "k8s-otel-container-insights", r.Labels.Instrumentation["cloudwatch.solution"], "%s on %s: cloudwatch.solution", metricName, pool)
			require.Equal(t, "dcgm", r.Labels.Instrumentation["cloudwatch.pipeline"], "%s on %s: cloudwatch.pipeline", metricName, pool)
		}
	})
}

// TestDCGMDeviceAttributes validates the per-GPU datapoint attributes DCGM attaches.
func TestDCGMDeviceAttributes(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			dp := r.Labels.Datapoint
			require.NotEmpty(t, dp["gpu"], "%s on %s: datapoint gpu", metricName, pool)
			require.True(t, strings.HasPrefix(dp["UUID"], "GPU-"), "%s on %s: UUID should start with GPU-, got %q", metricName, pool, dp["UUID"])
			require.True(t, strings.HasPrefix(dp["device"], "nvidia"), "%s on %s: device should start with nvidia, got %q", metricName, pool, dp["device"])
			require.NotEmpty(t, dp["modelName"], "%s on %s: datapoint modelName", metricName, pool)
			require.NotEmpty(t, dp["DCGM_FI_DRIVER_VERSION"], "%s on %s: datapoint DCGM_FI_DRIVER_VERSION", metricName, pool)
		}
	})
}

// TestDCGMAKSResourceAttributes validates the azure.aks resource attributes on GPU node metrics,
// with the same expected values as aksResourceExpectations in test/azure/aks.
func TestDCGMAKSResourceAttributes(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			res := r.Labels.Resource
			require.Equal(t, "azure", res["cloud.provider"], "%s on %s: cloud.provider", metricName, pool)
			require.Equal(t, "azure.aks", res["cloud.platform"], "%s on %s: cloud.platform", metricName, pool)
			require.Equal(t, env.AKSClusterName, res["k8s.cluster.name"], "%s on %s: k8s.cluster.name", metricName, pool)
			require.Equal(t, env.AzureLocation, res["cloud.region"], "%s on %s: cloud.region", metricName, pool)
			require.Equal(t, env.AzureResourceGroup, res["azure.resourcegroup.name"], "%s on %s: azure.resourcegroup.name", metricName, pool)
			require.NotEmpty(t, res["cloud.account.id"], "%s on %s: cloud.account.id", metricName, pool)
		}
	})
}

// TestDCGMHostAttributes validates the GPU host identity, the Azure counterpart of the EKS host.type
// (instance type) and host.id (i-…) checks: VM size, VM ID, and the VM and scale set of the pool.
func TestDCGMHostAttributes(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			res := r.Labels.Resource
			require.Equal(t, env.AzureVMSize, res["azure.vm.size"], "%s on %s: azure.vm.size", metricName, pool)
			require.NotEmpty(t, res["host.id"], "%s on %s: host.id", metricName, pool)
			require.Equal(t, res["k8s.node.name"], res["host.name"], "%s on %s: host.name should equal k8s.node.name", metricName, pool)
			require.True(t, strings.HasPrefix(res["azure.vm.name"], "aks-"+pool+"-"),
				"%s on %s: azure.vm.name should start with aks-%s-, got %q", metricName, pool, pool, res["azure.vm.name"])
			require.True(t, strings.HasPrefix(res["azure.vm.scaleset.name"], "aks-"+pool+"-"),
				"%s on %s: azure.vm.scaleset.name should start with aks-%s-, got %q", metricName, pool, pool, res["azure.vm.scaleset.name"])
		}
	})
}

// TestDCGMHostScrapePath validates the metrics come through the chart's dcgm-host scrape job (AKS host
// DCGM on :19400) and that the exporter's node_name matches the node the agent attributes them to.
func TestDCGMHostScrapePath(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			require.Equal(t, "dcgm-host", r.Labels.Resource["service.name"], "%s on %s: service.name", metricName, pool)
			require.Equal(t, r.Labels.Resource["k8s.node.name"], r.Labels.Datapoint["node_name"],
				"%s on %s: datapoint node_name should equal k8s.node.name", metricName, pool)
		}
	})
}

// TestDCGMNodeColor validates node labels from the node pool reach DCGM metrics.
func TestDCGMNodeColor(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			require.Equal(t, "green", r.Labels.Resource[nodeColorLabel], "%s on %s: %s", metricName, pool, nodeColorLabel)
		}
	})
}

// TestDCGMNoStaleDatapointKeys validates Hostname and pci_bus_id are removed from datapoints.
func TestDCGMNoStaleDatapointKeys(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			for _, key := range []string{"Hostname", "pci_bus_id"} {
				_, has := r.Labels.Datapoint[key]
				require.False(t, has, "%s on %s: datapoint %s should be removed", metricName, pool, key)
			}
		}
	})
}

// TestDCGMNoHwAttributes validates no hw.* resource attributes are attached.
func TestDCGMNoHwAttributes(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult) {
		for _, r := range results {
			for _, attr := range []string{"hw.type", "hw.vendor", "hw.model", "hw.name", "hw.id"} {
				_, has := r.Labels.Resource[attr]
				require.False(t, has, "%s on %s: unexpected @resource.%s", metricName, pool, attr)
			}
		}
	})
}
