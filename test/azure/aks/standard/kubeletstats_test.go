//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const scopeKubeletstats = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver"

var kubeletstatsMetricNamesList = metricNames(kubeletstatsMetrics)

func TestKubeletstatsMetricExistence(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
		})
	}
}

func TestKubeletstatsInstrumentationSource(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopeKubeletstats, name,
					"%s instrumentation name", metricName)
			}
		})
	}
}

func TestKubeletstatsClusterIdentity(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, cfg.ClusterName, r.Labels.Resource["k8s.cluster.name"],
					"%s cluster name", metricName)
			}
		})
	}
}

func TestKubeletstatsNodeName(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				node, ok := r.Labels.Resource["k8s.node.name"]
				require.True(t, ok, "%s missing @resource.k8s.node.name", metricName)
				require.True(t, node != "", "%s has empty @resource.k8s.node.name", metricName)
			}
		})
	}
}

func TestKubeletstatsPodName(t *testing.T) {
	var podAndContainerMetrics []string
	podAndContainerMetrics = append(podAndContainerMetrics, metricNames(kubeletstatsPodMetrics)...)
	podAndContainerMetrics = append(podAndContainerMetrics, metricNames(kubeletstatsContainerMetrics)...)
	for _, metricName := range podAndContainerMetrics {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				pod, ok := r.Labels.Resource["k8s.pod.name"]
				require.True(t, ok, "%s missing @resource.k8s.pod.name", metricName)
				require.True(t, pod != "", "%s has empty @resource.k8s.pod.name", metricName)
			}
		})
	}
}

func TestKubeletstatsNamespace(t *testing.T) {
	var podAndContainerMetrics []string
	podAndContainerMetrics = append(podAndContainerMetrics, metricNames(kubeletstatsPodMetrics)...)
	podAndContainerMetrics = append(podAndContainerMetrics, metricNames(kubeletstatsContainerMetrics)...)
	for _, metricName := range podAndContainerMetrics {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				ns, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing @resource.k8s.namespace.name", metricName)
				require.True(t, ns != "", "%s has empty @resource.k8s.namespace.name", metricName)
			}
		})
	}
}

func TestKubeletstatsContainerName(t *testing.T) {
	for _, md := range kubeletstatsContainerMetrics {
		t.Run(md.Name, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			hasContainer := 0
			for _, r := range results {
				if cn, ok := r.Labels.Resource["k8s.container.name"]; ok {
					require.True(t, cn != "", "%s has empty k8s.container.name", md.Name)
					hasContainer++
				}
			}
			require.True(t, hasContainer > 0,
				"%s has no results with k8s.container.name", md.Name)
		})
	}
}

// TestKubeletstatsCloudDetection is the Azure version of the EKS test, which asserts cloud.provider=aws, a
// region, and a 12-digit AWS account ID. On AKS the node agent reports provider azure, platform azure.aks,
// the configured location, and the subscription ID as the account ID.
func TestKubeletstatsCloudDetection(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName+"/provider", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, "azure", r.Labels.Resource["cloud.provider"], "%s cloud.provider", metricName)
			}
		})
		t.Run(metricName+"/platform", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, "azure.aks", r.Labels.Resource["cloud.platform"], "%s cloud.platform", metricName)
			}
		})
		t.Run(metricName+"/region", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, env.AzureLocation, r.Labels.Resource["cloud.region"], "%s cloud.region", metricName)
			}
		})
		t.Run(metricName+"/account_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.NotEmpty(t, r.Labels.Resource["cloud.account.id"], "%s cloud.account.id", metricName)
			}
		})
	}
}

// TestKubeletstatsHostDetection is the Azure version of the EKS test, which asserts an EC2 host.id ("i-..."),
// a host.name and a host.type. On AKS host.id is the VM identity (value not knowable at plan time, so
// presence only), host.name equals the node name, and azure.vm.size replaces host.type.
func TestKubeletstatsHostDetection(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName+"/host_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.NotEmpty(t, r.Labels.Resource["host.id"], "%s missing @resource.host.id", metricName)
			}
		})
		t.Run(metricName+"/host_name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.NotEmpty(t, r.Labels.Resource["host.name"], "%s missing @resource.host.name", metricName)
				require.Equal(t, r.Labels.Resource["k8s.node.name"], r.Labels.Resource["host.name"],
					"%s host.name should equal k8s.node.name", metricName)
			}
		})
		t.Run(metricName+"/vm_size", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, env.AzureVMSize, r.Labels.Resource["azure.vm.size"], "%s azure.vm.size", metricName)
			}
		})
	}
}

func TestKubeletstatsExpectedLabels(t *testing.T) {
	for _, md := range kubeletstatsMetrics {
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				for _, label := range md.ExpectedLabels {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s' (node: %s, azure.vm.size: %s, pod: %s)",
						md.Name, label,
						r.Labels.Resource["k8s.node.name"],
						r.Labels.Resource["azure.vm.size"],
						r.Labels.Resource["k8s.pod.name"])
				}
			}
		})
	}
}

func TestKubeletstatsUnitValidation(t *testing.T) {
	for _, md := range kubeletstatsMetrics {
		if md.Unit == "" {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				unit, ok := r.Labels.Datapoint["__unit__"]
				require.True(t, ok, "%s missing __unit__ label", md.Name)
				require.Equal(t, md.Unit, unit, "%s unit", md.Name)
			}
		})
	}
}

// TestKubeletstatsNodeGroupCoverage is the AKS version of the EKS node-group check, which filters by EC2
// host.type. The standard suite has one node pool, "default", whose nodes are named aks-default-*.
func TestKubeletstatsNodeGroupCoverage(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		results, err := queryCache.Get(context.Background(), "k8s.node.cpu.usage")
		require.NoError(t, err, "querying k8s.node.cpu.usage")
		found := 0
		for _, r := range results {
			if strings.HasPrefix(r.Labels.Resource["k8s.node.name"], "aks-default-") {
				found++
			}
		}
		require.True(t, found > 0,
			"kubeletstats missing from default pool nodes (aks-default-*) - kubeletstatsreceiver not scraping?")
	})
}

// TestKubeletstatsNodeNetworkIOProbe is an AKS-only diagnostic. The EKS suite leaves k8s.node.network.io out
// because kubelet on AL2023 does not emit it. This logs whether AKS does, and never fails.
func TestKubeletstatsNodeNetworkIOProbe(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "k8s.node.network.io")
	if err != nil {
		t.Logf("k8s.node.network.io query error: %v", err)
		return
	}
	t.Logf("k8s.node.network.io series on AKS: %d (0 means kubelet does not emit it here, as on EKS AL2023)", len(results))
}
