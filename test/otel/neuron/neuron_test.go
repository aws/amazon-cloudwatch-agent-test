//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

var neuronMetricNamesList = metricNames(neuronMetrics)

// neuronInstanceTypes lists instance types with Neuron devices in this cluster.
var neuronInstanceTypes = []struct {
	InstanceType string
	Description  string
}{
	{"inf2.xlarge", "Neuron"},
	{"inf2.24xlarge", "Neuron multi-device"},
}

// TestNeuronInstrumentationSource validates that all Neuron metrics
// have @instrumentation.@name == "awsneuron".
func TestNeuronInstrumentationSource(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, "awsneuron", name, "%s instrumentation name", metricName)
			}
		})
	}
}

// TestNeuronPodName validates that core-level metrics have pod correlation.
func TestNeuronPodName(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			var correlated []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					correlated = append(correlated, r)
				}
			}
			require.True(t, len(correlated) > 0,
				"No %s results have @resource.k8s.pod.name — are neuron-burn pods running?", md.Name)
			for _, r := range correlated {
				r := r
				require.True(t, r.Labels.Resource["k8s.pod.name"] != "",
					"%s has empty @resource.k8s.pod.name", md.Name)
				_, hasNS := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, hasNS, "%s correlated result missing @resource.k8s.namespace.name", md.Name)
			}
		})
	}
}

// TestNeuronNamespace validates that correlated Neuron results have namespace.
func TestNeuronNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					_, hasNS := r.Labels.Resource["k8s.namespace.name"]
					require.True(t, hasNS, "%s correlated result missing namespace", metricName)
				}
			}
		})
	}
}

// TestNeuronMultiNodeCoverage validates that neuroncore_utilization_ratio
// is reported from at least 2 Neuron nodes.
func TestNeuronMultiNodeCoverage(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available (no Neuron nodes?)")

	nodes := make(map[string]struct{})
	for _, r := range results {
		r := r
		if node, ok := r.Labels.Resource["k8s.node.name"]; ok {
			nodes[node] = struct{}{}
		}
	}
	require.True(t, len(nodes) >= 2,
		"Expected Neuron metrics from at least 2 nodes, got %d", len(nodes))
}

// TestNeuronActiveNodePodLabels validates that at least one result has
// "neuron-burn" in the pod name.
func TestNeuronActiveNodePodLabels(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available (no Neuron nodes?)")

	hasBurn := false
	for _, r := range results {
		r := r
		if pod, ok := r.Labels.Resource["k8s.pod.name"]; ok {
			if strings.Contains(pod, "neuron-burn") {
				hasBurn = true
				break
			}
		}
	}
	require.True(t, hasBurn, "No metrics found with neuron-burn pod name")
}

// TestNeuronDeviceAttributes validates core-level vs runtime-level device attributes.
func TestNeuronDeviceAttributes(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name+"/core_level", func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			for _, r := range results {
				r := r
				_, hasNC := r.Labels.Datapoint["aws.neuron.core"]
				require.True(t, hasNC, "%s missing datapoint aws.neuron.core", md.Name)
				_, hasND := r.Labels.Datapoint["aws.neuron.device"]
				require.True(t, hasND, "%s missing datapoint aws.neuron.device", md.Name)

				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; hasPod {
					_, hasCN := r.Labels.Resource["k8s.container.name"]
					require.True(t, hasCN,
						"%s correlated result missing @resource.k8s.container.name (pod: %s)",
						md.Name, r.Labels.Resource["k8s.pod.name"])
				}
			}
		})
	}

	for _, md := range neuronRuntimeLevelMetrics {
		md := md
		t.Run(md.Name+"/runtime_level", func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			for _, r := range results {
				r := r
				_, hasNC := r.Labels.Datapoint["aws.neuron.core"]
				require.True(t, !hasNC,
					"%s runtime-level metric should not have datapoint aws.neuron.core", md.Name)
				_, hasND := r.Labels.Datapoint["aws.neuron.device"]
				require.True(t, !hasND,
					"%s runtime-level metric should not have datapoint aws.neuron.device", md.Name)
			}
		})
	}
}

// TestNeuronRuntimeTagInResourceScope validates runtime_tag promotion.
func TestNeuronRuntimeTagInResourceScope(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			var correlated []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					correlated = append(correlated, r)
				}
			}
			require.True(t, len(correlated) > 0,
				"No correlated %s results to check runtime tag", md.Name)
			for _, r := range correlated {
				r := r
				tag, hasTag := r.Labels.Resource["aws.neuron.runtime.tag"]
				require.True(t, hasTag,
					"%s correlated result missing @resource.aws.neuron.runtime.tag (pod: %s)",
					md.Name, r.Labels.Resource["k8s.pod.name"])
				require.True(t, tag != "",
					"%s @resource.aws.neuron.runtime.tag is empty (pod: %s)",
					md.Name, r.Labels.Resource["k8s.pod.name"])
				_, hasDpTag := r.Labels.Datapoint["runtime_tag"]
				require.True(t, !hasDpTag,
					"%s should not have datapoint runtime_tag after promotion (pod: %s)",
					md.Name, r.Labels.Resource["k8s.pod.name"])
			}
		})
	}
}

// TestNeuronNoHwAttributes validates that NO Neuron results have hw.* attributes.
func TestNeuronNoHwAttributes(t *testing.T) {
	t.Parallel()
	hwAttrs := []string{"hw.type", "hw.vendor", "hw.model", "hw.name", "hw.id"}
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hwAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has,
						"%s should not have @resource.%s but got: %s",
						metricName, attr, r.Labels.Resource[attr])
				}
			}
		})
	}
}

// TestNeuronExpectedLabels validates expected datapoint labels.
func TestNeuronExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range neuronMetrics {
		md := md
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			for _, r := range results {
				r := r
				for _, label := range md.ExpectedLabels {
					label := label
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// TestNeuronBurnContainerName validates container name on burn pod results.
func TestNeuronBurnContainerName(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			var burn []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if strings.Contains(r.Labels.Resource["k8s.pod.name"], "neuron-burn") {
					burn = append(burn, r)
				}
			}
			require.True(t, len(burn) > 0, "No %s results from neuron-burn pods", md.Name)
			for _, r := range burn {
				r := r
				cn, ok := r.Labels.Resource["k8s.container.name"]
				require.True(t, ok,
					"%s neuron-burn result missing k8s.container.name (pod: %s)",
					md.Name, r.Labels.Resource["k8s.pod.name"])
				require.Equal(t, "neuron-burn", cn, "%s neuron-burn container name", md.Name)
			}
		})
	}
}

// TestNeuronIdleNodeNoContainerName validates uncorrelated results lack container name.
func TestNeuronIdleNodeNoContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			var uncorrelated []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
					uncorrelated = append(uncorrelated, r)
				}
			}
			require.NotEmpty(t, uncorrelated, "No uncorrelated results for %s", metricName)
			for _, r := range uncorrelated {
				r := r
				_, has := r.Labels.Resource["k8s.container.name"]
				require.True(t, !has,
					"%s idle Neuron node should not have k8s.container.name but got: %s",
					metricName, r.Labels.Resource["k8s.container.name"])
			}
		})
	}
}

// TestNeuronBurnWorkloadLabels validates workload labels on neuron-burn-core results.
func TestNeuronBurnWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			var burnCore []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				pod := r.Labels.Resource["k8s.pod.name"]
				if strings.HasPrefix(pod, "neuron-burn-core") {
					burnCore = append(burnCore, r)
				}
			}
			require.True(t, len(burnCore) > 0, "No %s results from neuron-burn-core pods", md.Name)
			for _, r := range burnCore {
				r := r
				require.Equal(t, "neuron-burn-core", r.Labels.Resource["k8s.workload.name"], "%s neuron-burn-core pod k8s.workload.name", md.Name)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "%s neuron-burn-core pod k8s.workload.type", md.Name)
				require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s neuron-burn-core pod k8s.namespace.name", md.Name)
				require.Equal(t, "neuron-burn-core", r.Labels.Resource["k8s.deployment.name"], "%s neuron-burn-core pod k8s.deployment.name", md.Name)
			}
		})
	}
}

// TestNeuronIdleNodeNoWorkloadLabels validates uncorrelated results lack workload labels.
func TestNeuronIdleNodeNoWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			var uncorrelated []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
					uncorrelated = append(uncorrelated, r)
				}
			}
			require.NotEmpty(t, uncorrelated, "No uncorrelated results for %s", metricName)
			for _, r := range uncorrelated {
				r := r
				_, hasName := r.Labels.Resource["k8s.workload.name"]
				require.True(t, !hasName,
					"%s idle Neuron node has unexpected k8s.workload.name: %s",
					metricName, r.Labels.Resource["k8s.workload.name"])
			}
		})
	}
}

// TestNeuronBurnCorePodColor validates pod-color=orange on neuron-burn-core results.
func TestNeuronBurnCorePodColor(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", md.Name)
			var burnCore []otelmetrics.MetricResult
			for _, r := range results {
				r := r
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "neuron-burn-core") {
					burnCore = append(burnCore, r)
				}
			}
			require.NotEmpty(t, burnCore, "%s: expected results from neuron-burn-core pods", md.Name)
			for _, r := range burnCore {
				r := r
				require.Equal(t, "orange", r.Labels.Resource[podColorLabel], "%s neuron-burn-core expected pod-color=orange", md.Name)
			}
		})
	}
}

// TestNeuronBurnDevicePodColor validates pod-color label propagation on the
// multi-device burn workload (neuron-burn-multi, color=violet).
func TestNeuronBurnDevicePodColor(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)

			found := false
			for _, r := range results {
				r := r
				pod, ok := r.Labels.Resource["k8s.pod.name"]
				if !ok {
					continue
				}
				if !strings.HasPrefix(pod, "neuron-burn-multi") {
					continue
				}
				color := r.Labels.Resource[podColorLabel]
				require.Equal(t, "violet", color, "%s: neuron-burn-multi should have pod-color=violet, got %q", md.Name, color)
				found = true
			}
			require.True(t, found,
				"%s: expected results from neuron-burn-multi pods", md.Name)
		})
	}
}

// TestNeuronNoInstanceTypeDatapoint validates instance_type absent from datapoint scope.
func TestNeuronNoInstanceTypeDatapoint(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Datapoint["instance_type"]
				require.True(t, !has,
					"%s should not have datapoint 'instance_type' after cleanup but got: %s",
					metricName, r.Labels.Datapoint["instance_type"])
			}
		})
	}
}

// TestNeuronNoPromotedDatapointKeys validates promoted keys absent from datapoint scope.
func TestNeuronNoPromotedDatapointKeys(t *testing.T) {
	t.Parallel()
	promotedKeys := []string{"k8s.pod.name", "k8s.namespace.name", "k8s.container.name"}
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				for _, key := range promotedKeys {
					key := key
					_, has := r.Labels.Datapoint[key]
					require.True(t, !has,
						"%s should not have datapoint '%s' after promotion but got: %s",
						metricName, key, r.Labels.Datapoint[key])
				}
			}
		})
	}
}

// TestNeuronIdleNodeNoPodColor validates uncorrelated results lack pod color label.
func TestNeuronIdleNodeNoPodColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			for _, r := range results {
				r := r
				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
					_, has := r.Labels.Resource[podColorLabel]
					require.True(t, !has,
						"%s idle Neuron node should not have %s", metricName, podColorLabel)
				}
			}
		})
	}
}

// TestNeuronNodeGroupCoverage validates Neuron metrics present on all Neuron node groups.
func TestNeuronNodeGroupCoverage(t *testing.T) {
	t.Parallel()
	escaped := escapePromQL(cfg.ClusterName)
	for _, ng := range neuronInstanceTypes {
		ng := ng
		t.Run(ng.Description+"/"+ng.InstanceType, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(
				`neuroncore_utilization_ratio{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
				escaped, ng.InstanceType)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying neuroncore_utilization_ratio on %s", ng.Description)
			require.True(t, len(results) > 0,
				"Neuron metrics missing from %s nodes (%s) — neuron-monitor not running?",
				ng.Description, ng.InstanceType)
		})
	}
}
