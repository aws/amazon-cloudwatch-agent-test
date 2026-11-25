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

// TestKubeletstatsCloudDetection is the GCP version of the EKS test, which asserts cloud.provider=aws, a
// region, and a 12-digit AWS account ID. On GKE the node agent reports provider gcp, platform
// gcp_kubernetes_engine, and the GCP project as the account ID -- the same gcp resourcedetection block
// requireGKECloudAttributes (ksm_test.go) asserts for KSM, but here on the node-agent pipeline rather than
// the cluster-scraper one. No region check: a real run's raw CloudWatch label dump (kube_node_info,
// 2026-11-24) confirmed cloud.region is absent from the resource attributes entirely, on both pipelines --
// not a wrong value, nothing is there to assert against. Only zone-level data reaches CloudWatch today
// (cloud.availability_zone, and separately k8s.node.label.topology.gke.io/zone).
func TestKubeletstatsCloudDetection(t *testing.T) {
	for _, metricName := range kubeletstatsMetricNamesList {
		t.Run(metricName+"/provider", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, "gcp", r.Labels.Resource["cloud.provider"], "%s cloud.provider", metricName)
			}
		})
		t.Run(metricName+"/platform", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, "gcp_kubernetes_engine", r.Labels.Resource["cloud.platform"], "%s cloud.platform", metricName)
			}
		})
		t.Run(metricName+"/account_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				require.Equal(t, env.GCPProject, r.Labels.Resource["cloud.account.id"], "%s cloud.account.id", metricName)
			}
		})
	}
}

// TestKubeletstatsHostDetection is the GCP version of the EKS test, which asserts an EC2 host.id ("i-..."),
// a host.name and a host.type. The gcp resourcedetection block on the node-agent pipeline enables host.id
// and host.name (unlike the cluster-scraper's KSM pipeline, which disables both), so those two assert the
// same way as on AKS. There is no GCP equivalent of host.type/azure.vm.size: the gcp detector has no
// machine-type attribute at all, so that sub-test stays skipped until the chart exposes one.
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
		t.Run(metricName+"/machine_type", func(t *testing.T) {
			t.Skip("No GCP equivalent of azure.vm.size/host.type: the gcp resourcedetection block has no " +
				"machine-type attribute for this pipeline; see the GKE standard-suite plan")
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
					require.True(t, ok, "%s missing expected label '%s' (node: %s, zone: %s, pod: %s)",
						md.Name, label,
						r.Labels.Resource["k8s.node.name"],
						r.Labels.Resource["cloud.availability_zone"],
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

// TestKubeletstatsNodeGroupCoverage is the GKE version of the EKS node-group check, which filters by EC2
// host.type. The standard suite's single node pool is GKE's implicit default: since
// google_container_cluster.cwagent sets no explicit node pool name, GKE names it "default-pool" and its
// nodes "gke-<cluster-name>-default-pool-<hash>-<id>" -- unlike AKS, where the pool's own name ("default")
// is a literal prefix, so this checks for the substring rather than a prefix.
func TestKubeletstatsNodeGroupCoverage(t *testing.T) {
	t.Run("default-pool", func(t *testing.T) {
		results, err := queryCache.Get(context.Background(), "k8s.node.cpu.usage")
		require.NoError(t, err, "querying k8s.node.cpu.usage")
		found := 0
		for _, r := range results {
			if strings.Contains(r.Labels.Resource["k8s.node.name"], "default-pool") {
				found++
			}
		}
		require.True(t, found > 0,
			"kubeletstats missing from default-pool nodes - kubeletstatsreceiver not scraping?")
	})
}

// TestKubeletstatsNodeNetworkIOProbe is a GKE-only diagnostic, same idea as the one AKS ran (and then
// removed once confirmed, commit 6c343797): the EKS suite leaves k8s.node.network.io out of
// kubeletstatsNodeMetrics because kubelet on AL2023 doesn't emit it, but AKS's kubelet (1.36) does. GKE's
// is unconfirmed either way. This logs whether it shows up and never fails; once a real run answers it,
// either move the metric into kubeletstatsNodeMetrics (metrics_test.go) and delete this probe, or delete
// just this probe if it stays absent like EKS.
func TestKubeletstatsNodeNetworkIOProbe(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "k8s.node.network.io")
	if err != nil {
		t.Logf("k8s.node.network.io query error: %v", err)
		return
	}
	t.Logf("k8s.node.network.io series on GKE: %d (0 means kubelet does not emit it here, as on EKS AL2023)", len(results))
}
