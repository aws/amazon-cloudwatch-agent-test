//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package standard contains cross-source label decoration tests for the standard cluster.
package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestClusterIdentity — every metric must identify the cluster.
// ---------------------------------------------------------------------------

func TestClusterIdentity(t *testing.T) {
	for _, metricName := range allMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.True(t, len(results) > 0,
				"%s returned 0 results when filtering by @resource.k8s.cluster.name=%s",
				metricName, cfg.ClusterName)
			for _, r := range results {
				require.Equal(t, cfg.ClusterName, r.Labels.Resource["k8s.cluster.name"],
					"%s cluster name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAWSLabels — every metric must have @aws.account and @aws.region.
// ---------------------------------------------------------------------------

func TestAWSLabels(t *testing.T) {
	for _, metricName := range allMetricNames() {
		t.Run(metricName+"/account", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				acct, ok := r.Labels.AWS["account"]
				require.True(t, ok, "%s missing @aws.account", metricName)
				require.Equal(t, 12, len(acct), "%s @aws.account length", metricName)
			}
		})
		t.Run(metricName+"/region", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				region, ok := r.Labels.AWS["region"]
				require.True(t, ok, "%s missing @aws.region", metricName)
				require.True(t, region != "", "%s has empty @aws.region", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestInstrumentationLabels — every metric must have instrumentation scope.
// ---------------------------------------------------------------------------

func TestInstrumentationLabels(t *testing.T) {
	for _, metricName := range allMetricNames() {
		t.Run(metricName+"/name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.True(t, name != "", "%s has empty @instrumentation.@name", metricName)
			}
		})
		t.Run(metricName+"/version", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				_, ok := r.Labels.Instrumentation["@version"]
				require.True(t, ok, "%s missing @instrumentation.@version", metricName)
			}
		})
		t.Run(metricName+"/cloudwatch_source", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				src, ok := r.Labels.Instrumentation["cloudwatch.source"]
				require.True(t, ok, "%s missing @instrumentation.cloudwatch.source", metricName)
				require.Equal(t, "cloudwatch-agent", src, "%s @instrumentation.cloudwatch.source", metricName)
			}
		})
		t.Run(metricName+"/cloudwatch_solution", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				sol, ok := r.Labels.Instrumentation["cloudwatch.solution"]
				require.True(t, ok, "%s missing @instrumentation.cloudwatch.solution", metricName)
				require.Equal(t, "k8s-otel-container-insights", sol, "%s @instrumentation.cloudwatch.solution", metricName)
			}
		})
		t.Run(metricName+"/cloudwatch_pipeline", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				_, ok := r.Labels.Instrumentation["cloudwatch.pipeline"]
				require.True(t, ok, "%s missing @instrumentation.cloudwatch.pipeline", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestScopeNameAndVersionPerSource — standard cluster sources only.
// ---------------------------------------------------------------------------

func TestScopeNameAndVersionPerSource(t *testing.T) {
	gt := getGroundTruth(t)

	agentVersion := imageTagFromPod(t, gt, "amazon-cloudwatch", "app.kubernetes.io/name", "cloudwatch-agent")
	if agentVersion == "latest" || agentVersion == "" || len(agentVersion) >= 32 {
		results, err := queryCache.Get(context.Background(), "container_cpu_usage_seconds_total")
		if err == nil && len(results) > 0 {
			agentVersion = results[0].Labels.Instrumentation["@version"]
		}
	}
	nodeExporterVersion := imageTagFromPod(t, gt, "amazon-cloudwatch", "k8s-app", "node-exporter")
	ksmVersion := imageTagFromPod(t, gt, "amazon-cloudwatch", "app.kubernetes.io/component", "kube-state-metrics")
	controlPlaneVersion := k8sServerVersion(t)

	t.Logf("agent=%s node_exporter=%s ksm=%s k8s=%s",
		agentVersion, nodeExporterVersion, ksmVersion, controlPlaneVersion)

	tests := []struct {
		source, metric, wantName, wantVersion, wantPipeline string
	}{
		{"node_exporter", "node_cpu_seconds_total", "github.com/prometheus/node_exporter", nodeExporterVersion, "node-exporter"},
		{"cadvisor", "container_cpu_usage_seconds_total", "github.com/google/cadvisor", agentVersion, "cadvisor"},
		{"kubeletstats", "k8s.node.cpu.utilization", scopeKubeletstats, agentVersion, "kubeletstats"},
		{"control_plane", "apiserver_request_total", scopePrometheus, controlPlaneVersion, "apiserver"},
		{"kube_state_metrics", "kube_deployment_status_replicas_ready", "github.com/kubernetes/kube-state-metrics", ksmVersion, "kube-state-metrics"},
	}

	for _, tc := range tests {
		t.Run(tc.source+"/name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), tc.metric)
			require.NoError(t, err, "querying %s", tc.metric)
			require.NotEmpty(t, results, "%s not available", tc.metric)
			require.Equal(t, tc.wantName, results[0].Labels.Instrumentation["@name"], "%s scope name", tc.source)
		})
		t.Run(tc.source+"/version", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), tc.metric)
			require.NoError(t, err, "querying %s", tc.metric)
			require.NotEmpty(t, results, "%s not available", tc.metric)
			require.Equal(t, tc.wantVersion, results[0].Labels.Instrumentation["@version"], "%s scope version", tc.source)
		})
		t.Run(tc.source+"/pipeline", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), tc.metric)
			require.NoError(t, err, "querying %s", tc.metric)
			require.NotEmpty(t, results, "%s not available", tc.metric)
			pipeline, ok := results[0].Labels.Instrumentation["cloudwatch.pipeline"]
			require.True(t, ok, "%s missing @instrumentation.cloudwatch.pipeline", tc.source)
			require.Equal(t, tc.wantPipeline, pipeline, "%s cloudwatch.pipeline", tc.source)
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudResourceDetection — cloud.* resource attributes from resourcedetection.
// ---------------------------------------------------------------------------

func TestCloudResourceDetection(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName+"/provider", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, "aws", r.Labels.Resource["cloud.provider"], "%s cloud.provider", metricName)
			}
		})
		t.Run(metricName+"/platform", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				platform := r.Labels.Resource["cloud.platform"]
				require.True(t, platform == "aws_eks" || platform == "aws_ec2",
					"%s cloud.platform=%q, want aws_eks or aws_ec2", metricName, platform)
			}
		})
		t.Run(metricName+"/region", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				region, ok := r.Labels.Resource["cloud.region"]
				require.True(t, ok, "%s missing @resource.cloud.region", metricName)
				require.True(t, region != "", "%s has empty @resource.cloud.region", metricName)
			}
		})
		t.Run(metricName+"/availability_zone", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				az, ok := r.Labels.Resource["cloud.availability_zone"]
				require.True(t, ok, "%s missing @resource.cloud.availability_zone", metricName)
				require.True(t, az != "", "%s has empty @resource.cloud.availability_zone", metricName)
			}
		})
		t.Run(metricName+"/account_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				acctID, ok := r.Labels.Resource["cloud.account.id"]
				require.True(t, ok, "%s missing @resource.cloud.account.id", metricName)
				require.Equal(t, 12, len(acctID), "%s cloud.account.id length", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestHostResourceDetection — host.* resource attributes from resourcedetection.
// ---------------------------------------------------------------------------

func TestHostResourceDetection(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName+"/host_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				hostID, ok := r.Labels.Resource["host.id"]
				require.True(t, ok, "%s missing @resource.host.id", metricName)
				require.True(t, strings.HasPrefix(hostID, "i-"),
					"%s host.id should start with 'i-', got '%s'", metricName, hostID)
			}
		})
		t.Run(metricName+"/host_type", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				hostType, ok := r.Labels.Resource["host.type"]
				require.True(t, ok, "%s missing @resource.host.type", metricName)
				require.True(t, hostType != "", "%s has empty @resource.host.type", metricName)
			}
		})
		t.Run(metricName+"/host_name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				hostName, ok := r.Labels.Resource["host.name"]
				require.True(t, ok, "%s missing @resource.host.name", metricName)
				require.True(t, hostName != "", "%s has empty @resource.host.name", metricName)
			}
		})
		t.Run(metricName+"/host_image_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				imageID, ok := r.Labels.Resource["host.image.id"]
				require.True(t, ok, "%s missing @resource.host.image.id", metricName)
				require.True(t, strings.HasPrefix(imageID, "ami-"),
					"%s host.image.id should start with 'ami-', got '%s'", metricName, imageID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNodeMetricLabels — k8s.node.name on DaemonSet metrics, k8s.node.uid on enriched.
// ---------------------------------------------------------------------------

func TestNodeMetricLabels(t *testing.T) {
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName+"/node_name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				nodeName, ok := r.Labels.Resource["k8s.node.name"]
				require.True(t, ok, "%s missing @resource.k8s.node.name (host: %s)",
					metricName, r.Labels.Resource["host.name"])
				require.True(t, nodeName != "", "%s has empty @resource.k8s.node.name", metricName)
			}
		})
	}

	for _, metricName := range nodeLabelEnrichedNames() {
		t.Run(metricName+"/node_uid", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				uid, ok := r.Labels.Resource["k8s.node.uid"]
				require.True(t, ok, "%s missing @resource.k8s.node.uid (node: %s)",
					metricName, r.Labels.Resource["k8s.node.name"])
				require.True(t, uid != "", "%s has empty @resource.k8s.node.uid", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPodMetricLabels — cadvisor strict pod labels.
// Standard cluster has no device metrics, so only cadvisor section applies.
// ---------------------------------------------------------------------------

func TestPodMetricLabels(t *testing.T) {
	for _, metricName := range podScopedCadvisorNames() {
		t.Run(metricName+"/cadvisor_strict", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				_, hasPod := r.Labels.Resource["k8s.pod.name"]
				require.True(t, hasPod, "%s missing @resource.k8s.pod.name", metricName)
				_, hasNS := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, hasNS, "%s missing @resource.k8s.namespace.name", metricName)
				_, hasNode := r.Labels.Resource["k8s.node.name"]
				require.True(t, hasNode, "%s missing @resource.k8s.node.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestContainerMetricLabels — container-scoped metrics must have container name.
// ---------------------------------------------------------------------------

func TestContainerMetricLabels(t *testing.T) {
	names := containerMetricNames()
	require.True(t, len(names) > 0, "No container-scoped metrics defined in test suite")
	for _, metricName := range names {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			hasContainer := 0
			for _, r := range results {
				if cn, ok := r.Labels.Resource["k8s.container.name"]; ok {
					require.True(t, cn != "", "%s has empty k8s.container.name", metricName)
					hasContainer++
				} else {
					_, hasPod := r.Labels.Resource["k8s.pod.name"]
					require.True(t, hasPod,
						"%s result without k8s.container.name also missing k8s.pod.name", metricName)
				}
			}
			require.True(t, hasContainer > 0, "%s has no results with k8s.container.name", metricName)
		})
	}
}

// ---------------------------------------------------------------------------
// TestLabelPreservation — datapoint labels not empty for all metrics.
// ---------------------------------------------------------------------------

func TestLabelPreservation(t *testing.T) {
	for _, metricName := range allMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.True(t, len(r.Labels.Datapoint) > 0, "%s has empty datapoint labels", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMetricNamePreservation — metric_name matches query name.
// ---------------------------------------------------------------------------

func TestMetricNamePreservation(t *testing.T) {
	for _, metricName := range allMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, metricName, r.MetricName, "%s metric name mismatch", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestServiceLabels — service.instance.id for Prometheus-scraped metrics.
// ---------------------------------------------------------------------------

func TestServiceLabels(t *testing.T) {
	for _, metricName := range prometheusScrapedNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				svcID, ok := r.Labels.Resource["service.instance.id"]
				require.True(t, ok, "%s missing @resource.service.instance.id", metricName)
				require.True(t, svcID != "", "%s has empty @resource.service.instance.id", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNodeLabelEnrichment — k8s.node.label.* present, NFD labels dropped.
// ---------------------------------------------------------------------------

func TestNodeLabelEnrichment(t *testing.T) {
	expectedNodeLabels := []string{
		"k8s.node.label.kubernetes.io/os",
		"k8s.node.label.kubernetes.io/arch",
	}

	for _, metricName := range nodeLabelEnrichedNames() {
		t.Run(metricName+"/standard_labels", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				// Only check nodes belonging to this cluster's color group.
				// Attr-limit nodes (white) may have these labels dropped by design.
				color := r.Labels.Resource[nodeColorLabel]
				if _, known := nodeColorToInstanceTypes[color]; !known {
					continue
				}
				for _, label := range expectedNodeLabels {
					_, ok := r.Labels.Resource[label]
					require.True(t, ok,
						"%s missing @resource.%s on node %s",
						metricName, label, r.Labels.Resource["k8s.node.name"])
				}
			}
		})

		t.Run(metricName+"/nfd_labels_dropped", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				var nfdLabels []string
				for k := range r.Labels.Resource {
					if strings.Contains(k, "feature.node.kubernetes.io") {
						nfdLabels = append(nfdLabels, k)
					}
				}
				require.True(t, len(nfdLabels) == 0,
					"%s has NFD labels that should have been dropped: %v", metricName, nfdLabels)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWorkloadLabels — standard cluster: node_exporter has no workload,
// cadvisor nginx-test has Deployment workload.
// ---------------------------------------------------------------------------

func TestWorkloadLabels(t *testing.T) {
	// Node-level: node_exporter metrics must NOT have workload labels.
	for _, metricName := range metricNames(nodeExporterMetrics) {
		t.Run(metricName+"/no_workload", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				_, hasName := r.Labels.Resource["k8s.workload.name"]
				require.True(t, !hasName,
					"%s node-level metric should not have k8s.workload.name but got: %s",
					metricName, r.Labels.Resource["k8s.workload.name"])
				_, hasType := r.Labels.Resource["k8s.workload.type"]
				require.True(t, !hasType,
					"%s node-level metric should not have k8s.workload.type but got: %s",
					metricName, r.Labels.Resource["k8s.workload.type"])
			}
		})
	}

	// Cadvisor: nginx-test is a Deployment.
	for _, metricName := range metricNames(cadvisorMetrics) {
		t.Run(metricName+"/cadvisor_nginx_workload", func(t *testing.T) {
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"nginx-test.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s for nginx-test", metricName)
			require.True(t, len(results) > 0, "No %s results from nginx-test pods", metricName)
			for _, r := range results {
				require.Equal(t, "nginx-test", r.Labels.Resource["k8s.workload.name"],
					"%s nginx-test pod k8s.workload.name", metricName)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"],
					"%s nginx-test pod k8s.workload.type", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMetricTypeLabels — metric type metadata labels match expected types.
// ---------------------------------------------------------------------------

func TestMetricTypeLabels(t *testing.T) {
	for _, md := range allMetrics {
		if md.MetricType != "counter" {
			continue
		}
		t.Run(md.Name+"/counter_type", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, "Sum", r.Labels.Datapoint["__type__"], "%s __type__", md.Name)
				require.Equal(t, "true", r.Labels.Datapoint["__monotonicity__"], "%s __monotonicity__", md.Name)
				require.Equal(t, "cumulative", r.Labels.Datapoint["__temporality__"], "%s __temporality__", md.Name)
			}
		})
	}

	for _, md := range allMetrics {
		if md.MetricType != "gauge" {
			continue
		}
		t.Run(md.Name+"/gauge_type", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, "Gauge", r.Labels.Datapoint["__type__"], "%s __type__", md.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMetricUnitLabels — __unit__ metadata label matches expected unit.
// ---------------------------------------------------------------------------

func TestMetricUnitLabels(t *testing.T) {
	for _, md := range allMetrics {
		if md.Unit == "" {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				unit, ok := r.Labels.Datapoint["__unit__"]
				require.True(t, ok, "%s missing __unit__ datapoint label", md.Name)
				require.Equal(t, md.Unit, unit, "%s __unit__", md.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCustomNodeLabels — custom node label preserved on enriched metrics.
// Adapted for standard cluster: skips if the label doesn't exist.
// ---------------------------------------------------------------------------

func TestCustomNodeLabels(t *testing.T) {
	validColors := map[string]struct{}{
		"blue": {}, "green": {}, "red": {}, "yellow": {}, "white": {},
	}

	for _, metricName := range nodeLabelEnrichedNames() {
		t.Run(metricName+"/color_present", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				color := r.Labels.Resource[nodeColorLabel]
				if color == "" {
					continue
				}
				_, valid := validColors[color]
				require.True(t, valid,
					"%s unexpected node color '%s' (node: %s)",
					metricName, color, r.Labels.Resource["k8s.node.name"])
			}
		})
	}

	// node_exporter metrics must include at least one result with color=blue.
	for _, metricName := range metricNames(nodeExporterMetrics) {
		t.Run(metricName+"/has_blue", func(t *testing.T) {
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.%s"="blue"}`,
				metricName, cfg.ClusterName, nodeColorLabel)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s with blue filter", metricName)
			require.True(t, len(results) > 0,
				"%s no results from standard nodes (color=blue)", metricName)
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudResourceId — cloud.resource_id set to EKS cluster ARN.
// ---------------------------------------------------------------------------

func TestCloudResourceId(t *testing.T) {
	expectedPrefix := "arn:aws:eks:"
	expectedSuffix := ":cluster/" + cfg.ClusterName

	for _, metricName := range allMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				arn, ok := r.Labels.Resource["cloud.resource_id"]
				require.True(t, ok, "%s missing @resource.cloud.resource_id", metricName)
				require.True(t, strings.HasPrefix(arn, expectedPrefix),
					"%s cloud.resource_id should start with %q, got %q", metricName, expectedPrefix, arn)
				require.True(t, strings.HasSuffix(arn, expectedSuffix),
					"%s cloud.resource_id should end with %q, got %q", metricName, expectedSuffix, arn)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestScrapeMetadataFiltered — scrape metadata metrics must NOT appear from
// filtered pipelines.
// ---------------------------------------------------------------------------

func TestScrapeMetadataFiltered(t *testing.T) {
	scrapeMetrics := []string{
		"scrape_duration_seconds",
		"scrape_samples_scraped",
		"scrape_samples_post_metric_relabeling",
		"scrape_series_added",
		"up",
	}

	filteredPipelines := map[string]bool{
		"cadvisor":           true,
		"apiserver":          true,
		"kube-state-metrics": true,
		"node-exporter":      true,
	}

	for _, metricName := range scrapeMetrics {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.GetUnfiltered(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				pipeline := r.Labels.Instrumentation["cloudwatch.pipeline"]
				if filteredPipelines[pipeline] {
					t.Errorf("%s should be filtered from %s pipeline but was present", metricName, pipeline)
				}
			}
		})
	}
}
