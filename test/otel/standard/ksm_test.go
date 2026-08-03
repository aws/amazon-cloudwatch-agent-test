//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// =============================================================================
// KSM METRIC BUCKETS
// =============================================================================

var ksmNodeBucket = []string{
	"kube_node_info",
	"kube_node_status_condition",
	"kube_node_status_allocatable",
	"kube_node_status_capacity",
}

var ksmPodBucket = []string{
	"kube_pod_status_phase",
}

var ksmContainerBucket = []string{
	"kube_pod_container_status_running",
}

var ksmWorkloadBucket = struct {
	Deployment  []string
	DaemonSet   []string
	StatefulSet []string
	Job         []string
	CronJob     []string
}{
	Deployment:  []string{"kube_deployment_status_replicas", "kube_deployment_status_replicas_ready"},
	DaemonSet:   []string{"kube_daemonset_status_desired_number_scheduled"},
	StatefulSet: []string{"kube_statefulset_replicas", "kube_statefulset_status_replicas_ready"},
	Job:         []string{"kube_job_status_active"},
	CronJob:     []string{"kube_cronjob_status_active"},
}

func ksmWorkloadMetrics() []string {
	var all []string
	all = append(all, ksmWorkloadBucket.Deployment...)
	all = append(all, ksmWorkloadBucket.DaemonSet...)
	all = append(all, ksmWorkloadBucket.StatefulSet...)
	all = append(all, ksmWorkloadBucket.Job...)
	all = append(all, ksmWorkloadBucket.CronJob...)
	return all
}

var ksmClusterBucket = []string{
	"kube_namespace_status_phase",
}

var ksmNodeScopedBucketMetrics = func() []string {
	var all []string
	all = append(all, ksmNodeBucket...)
	all = append(all, ksmPodBucket...)
	all = append(all, ksmContainerBucket...)
	return all
}()

var ksmNonNodeMetrics = func() []string {
	var all []string
	all = append(all, ksmWorkloadMetrics()...)
	all = append(all, ksmClusterBucket...)
	return all
}()

func allKSMBucketMetrics() []string {
	var all []string
	all = append(all, ksmNodeBucket...)
	all = append(all, ksmPodBucket...)
	all = append(all, ksmContainerBucket...)
	all = append(all, ksmWorkloadMetrics()...)
	all = append(all, ksmClusterBucket...)
	return all
}

// allKSMMetricDefs combines both KSM MetricDefinition slices for ExpectedLabels/Unit tests.
var allKSMMetricDefs = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, ksmNodeScopedMetrics...)
	all = append(all, ksmClusterScopedMetrics...)
	return all
}()

// parseAZFromProviderID extracts the AZ from a K8s node provider ID.
// Format: "aws:///us-east-1a/i-0abc123def456" → "us-east-1a"
func parseAZFromProviderID(providerID string) (string, error) {
	parts := strings.Split(providerID, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("provider ID %q: not enough segments", providerID)
	}
	az := parts[len(parts)-2]
	if az == "" {
		return "", fmt.Errorf("provider ID %q: AZ segment is empty", providerID)
	}
	return az, nil
}

// =============================================================================
// COMMON TESTS (ALL BUCKETS)
// =============================================================================

func TestKSM_AllBuckets_MetricExists(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
		})
	}
}

func TestKSM_AllBuckets_InstrumentationSource(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, "github.com/kubernetes/kube-state-metrics", name, "%s", metricName)
			}
		})
	}
}

func TestKSM_AllBuckets_InstrumentationVersion(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				version, ok := r.Labels.Instrumentation["@version"]
				require.True(t, ok, "%s missing @instrumentation.@version", metricName)
				require.True(t, version != "", "%s has empty @instrumentation.@version", metricName)
			}
		})
	}
}

func TestKSM_AllBuckets_InstrumentationPipeline(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				pipeline, ok := r.Labels.Instrumentation["cloudwatch.pipeline"]
				require.True(t, ok, "%s missing @instrumentation.cloudwatch.pipeline", metricName)
				require.Equal(t, "kube-state-metrics", pipeline, "%s cloudwatch.pipeline", metricName)
			}
		})
	}
}

func TestKSM_AllBuckets_ClusterName(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				require.Equal(t, cfg.ClusterName, r.Labels.Resource["k8s.cluster.name"], "%s", metricName)
			}
		})
	}
}

func TestKSM_AllBuckets_NoComponentLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.component.name"]
				require.True(t, !has, "%s should not have k8s.component.name", metricName)
			}
		})
	}
}

// =============================================================================
// BUCKET 1: NODE METRICS
// =============================================================================

func TestKSM_NodeBucket_HasNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.node.name"]
				require.True(t, ok, "%s missing k8s.node.name", metricName)
				require.True(t, val != "", "%s has empty k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_NodeBucket_HasHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					val, ok := r.Labels.Resource[attr]
					require.True(t, ok, "%s missing %s", metricName, attr)
					require.True(t, val != "", "%s has empty %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_NodeBucket_HostImageIDStartsWithAMI(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				imageID := r.Labels.Resource["host.image.id"]
				require.True(t, strings.HasPrefix(imageID, "ami-"),
					"%s host.image.id=%s should start with ami-", metricName, imageID)
			}
		})
	}
}

func TestKSM_NodeBucket_AZMatchesProviderID(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				nodeName := r.Labels.Resource["k8s.node.name"]
				az := r.Labels.Resource["cloud.availability_zone"]
				if nodeName == "" || az == "" {
					continue
				}
				node, found := gt.nodes[nodeName]
				if !found {
					continue
				}
				expectedAZ, err := parseAZFromProviderID(node.Spec.ProviderID)
				if err != nil {
					continue
				}
				require.Equal(t, expectedAZ, az, "%s node %s AZ mismatch", metricName, nodeName)
			}
		})
	}
}

func TestKSM_NodeBucket_HasNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasNodeLabel := false
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						hasNodeLabel = true
						break
					}
				}
				if hasNodeLabel {
					break
				}
			}
			require.True(t, hasNodeLabel, "%s should have k8s.node.label.* attributes", metricName)
		})
	}
}

func TestKSM_NodeBucket_NoPodAttributes(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.pod.name"]
				require.True(t, !has, "%s should not have k8s.pod.name", metricName)
			}
		})
	}
}

func TestKSM_NodeBucket_NoWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmNodeBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, hasName := r.Labels.Resource["k8s.workload.name"]
				require.True(t, !hasName, "%s should not have k8s.workload.name", metricName)
				_, hasType := r.Labels.Resource["k8s.workload.type"]
				require.True(t, !hasType, "%s should not have k8s.workload.type", metricName)
			}
		})
	}
}

// =============================================================================
// BUCKET 2: POD METRICS
// =============================================================================

func TestKSM_PodBucket_HasPodIdentity(t *testing.T) {
	t.Parallel()
	requiredAttrs := []string{"k8s.pod.name", "k8s.namespace.name", "k8s.pod.uid"}
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				for _, attr := range requiredAttrs {
					attr := attr
					val, ok := r.Labels.Resource[attr]
					require.True(t, ok, "%s missing %s", metricName, attr)
					require.True(t, val != "", "%s has empty %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_PodBucket_HasNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasNodeName := false
			for _, r := range results {
				r := r
				if val, ok := r.Labels.Resource["k8s.node.name"]; ok && val != "" {
					hasNodeName = true
					break
				}
			}
			require.True(t, hasNodeName, "%s should have k8s.node.name on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasHost := false
			for _, r := range results {
				r := r
				if _, ok := r.Labels.Resource["host.id"]; ok {
					hasHost = true
					for _, attr := range hostAttrs {
						attr := attr
						val := r.Labels.Resource[attr]
						require.True(t, val != "", "%s has empty %s", metricName, attr)
					}
				}
			}
			require.True(t, hasHost, "%s should have host.* on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasCloudAZ(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasAZ := false
			for _, r := range results {
				r := r
				if az, ok := r.Labels.Resource["cloud.availability_zone"]; ok && az != "" {
					hasAZ = true
					break
				}
			}
			require.True(t, hasAZ, "%s should have cloud.availability_zone on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			workloadCount := 0
			for _, r := range results {
				r := r
				if wn, ok := r.Labels.Resource["k8s.workload.name"]; ok && wn != "" {
					workloadCount++
					wt := r.Labels.Resource["k8s.workload.type"]
					require.True(t, wt != "", "%s has k8s.workload.name but empty k8s.workload.type", metricName)
				}
			}
			require.True(t, workloadCount > 0, "%s should have k8s.workload.* on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasSpecificWorkloadTypeAttr(t *testing.T) {
	t.Parallel()
	workloadAttrs := []string{
		"k8s.deployment.name", "k8s.statefulset.name", "k8s.daemonset.name",
		"k8s.replicaset.name", "k8s.job.name", "k8s.cronjob.name",
	}
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasSpecific := false
			for _, r := range results {
				r := r
				for _, attr := range workloadAttrs {
					attr := attr
					if val, ok := r.Labels.Resource[attr]; ok && val != "" {
						hasSpecific = true
						break
					}
				}
				if hasSpecific {
					break
				}
			}
			require.True(t, hasSpecific, "%s should have k8s.<workload>.name on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_NginxDeploymentName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"nginx-test.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(ctx, promql)
			require.NoError(t, err, "querying %s for nginx-test", metricName)
			require.True(t, len(results) > 0, "No %s results from nginx-test pods", metricName)
			for _, r := range results {
				r := r
				require.Equal(t, "nginx-test", r.Labels.Resource["k8s.deployment.name"], "%s nginx-test pod k8s.deployment.name", metricName)
				require.Equal(t, "nginx-test", r.Labels.Resource["k8s.workload.name"], "%s nginx-test pod k8s.workload.name", metricName)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "%s nginx-test pod k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_PodBucket_HasNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasNodeLabel := false
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						hasNodeLabel = true
						break
					}
				}
				if hasNodeLabel {
					break
				}
			}
			require.True(t, hasNodeLabel, "%s should have k8s.node.label.* on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasPodLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasPodLabel := false
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.pod.label.") {
						hasPodLabel = true
						break
					}
				}
				if hasPodLabel {
					break
				}
			}
			require.True(t, hasPodLabel, "%s should have k8s.pod.label.* on at least some results", metricName)
		})
	}
}

func TestKSM_PodBucket_HasRawGroupByLabels(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "uid"}
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				for _, key := range rawKeys {
					key := key
					val, has := r.Labels.Datapoint[key]
					require.True(t, has, "%s missing raw '%s' label at datapoint scope", metricName, key)
					require.True(t, val != "", "%s has empty raw '%s' label at datapoint scope", metricName, key)
				}
			}
		})
	}
}

// Validate k8s.statefulset.name on ksm-test-statefulset pods.
func TestKSM_PodBucket_StatefulSetPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"ksm-test-statefulset.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s for ksm-test-statefulset", metricName)
			require.True(t, len(results) > 0, "No %s results from ksm-test-statefulset pods", metricName)
			for _, r := range results {
				r := r
				require.Equal(t, "ksm-test-statefulset", r.Labels.Resource["k8s.statefulset.name"], "%s k8s.statefulset.name", metricName)
				require.Equal(t, "ksm-test-statefulset", r.Labels.Resource["k8s.workload.name"], "%s k8s.workload.name", metricName)
				require.Equal(t, "StatefulSet", r.Labels.Resource["k8s.workload.type"], "%s k8s.workload.type", metricName)
			}
		})
	}
}

// Validate k8s.job.name on ksm-test-job pods.
func TestKSM_PodBucket_JobPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"ksm-test-job.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s for ksm-test-job", metricName)
			require.True(t, len(results) > 0, "No %s results from ksm-test-job pods", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.job.name"]
				require.True(t, ok && val != "", "%s ksm-test-job pod missing k8s.job.name", metricName)
				require.Equal(t, "Job", r.Labels.Resource["k8s.workload.type"], "%s k8s.workload.type", metricName)
			}
		})
	}
}

// Validate k8s.cronjob.name on ksm-test-cronjob pods.
func TestKSM_PodBucket_CronJobPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"ksm-test-cronjob.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s for ksm-test-cronjob", metricName)
			if len(results) == 0 {
				t.Skipf("No %s results from ksm-test-cronjob pods (may have been cleaned up)", metricName)
			}
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.cronjob.name"]
				require.True(t, ok && val != "", "%s ksm-test-cronjob pod missing k8s.cronjob.name", metricName)
				require.Equal(t, "ksm-test-cronjob", val, "%s k8s.cronjob.name", metricName)
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wt == "Job" || wt == "CronJob",
					"%s k8s.workload.type: got %s, want Job or CronJob", metricName, wt)
			}
		})
	}
}

// Validate k8s.replicaset.name on standalone ksm-test-replicaset pods (no Deployment parent).
func TestKSM_PodBucket_ReplicaSetPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmPodBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"ksm-test-replicaset.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s for ksm-test-replicaset", metricName)
			require.True(t, len(results) > 0, "No %s results from ksm-test-replicaset pods", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.replicaset.name"]
				require.True(t, ok && val != "", "%s ksm-test-replicaset pod missing k8s.replicaset.name", metricName)
				require.Equal(t, "ksm-test-replicaset", r.Labels.Resource["k8s.workload.name"], "%s k8s.workload.name", metricName)
				require.Equal(t, "ReplicaSet", r.Labels.Resource["k8s.workload.type"], "%s k8s.workload.type", metricName)
				_, hasDeploy := r.Labels.Resource["k8s.deployment.name"]
				require.True(t, !hasDeploy,
					"%s standalone ReplicaSet should NOT have k8s.deployment.name", metricName)
			}
		})
	}
}

// =============================================================================
// BUCKET 3: CONTAINER METRICS
// =============================================================================

func TestKSM_ContainerBucket_HasRawGroupByLabels(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "uid", "container"}
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				for _, key := range rawKeys {
					key := key
					val, has := r.Labels.Datapoint[key]
					require.True(t, has, "%s missing raw '%s' label at datapoint scope", metricName, key)
					require.True(t, val != "", "%s has empty raw '%s' label at datapoint scope", metricName, key)
				}
			}
		})
	}
}

func TestKSM_ContainerBucket_HasContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.container.name"]
				require.True(t, ok, "%s missing k8s.container.name", metricName)
				require.True(t, val != "", "%s has empty k8s.container.name", metricName)
			}
		})
	}
}

func TestKSM_ContainerBucket_HasPodIdentity(t *testing.T) {
	t.Parallel()
	requiredAttrs := []string{"k8s.pod.name", "k8s.namespace.name"}
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				for _, attr := range requiredAttrs {
					attr := attr
					val, ok := r.Labels.Resource[attr]
					require.True(t, ok, "%s missing %s", metricName, attr)
					require.True(t, val != "", "%s has empty %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_ContainerBucket_HasHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasHost := false
			for _, r := range results {
				r := r
				if _, ok := r.Labels.Resource["host.id"]; ok {
					hasHost = true
					for _, attr := range hostAttrs {
						attr := attr
						val := r.Labels.Resource[attr]
						require.True(t, val != "", "%s has empty %s", metricName, attr)
					}
				}
			}
			require.True(t, hasHost, "%s should have host.* attributes", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasCloudAZ(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasAZ := false
			for _, r := range results {
				r := r
				if az, ok := r.Labels.Resource["cloud.availability_zone"]; ok && az != "" {
					hasAZ = true
					break
				}
			}
			require.True(t, hasAZ, "%s should have cloud.availability_zone on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasNodeLabel := false
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						hasNodeLabel = true
						break
					}
				}
				if hasNodeLabel {
					break
				}
			}
			require.True(t, hasNodeLabel, "%s should have k8s.node.label.* on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasRawContainerLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["container"]
				require.True(t, has, "%s missing raw 'container' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'container' label", metricName)
			}
		})
	}
}

func TestKSM_ContainerBucket_HasNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasNodeName := false
			for _, r := range results {
				r := r
				if val, ok := r.Labels.Resource["k8s.node.name"]; ok && val != "" {
					hasNodeName = true
					break
				}
			}
			require.True(t, hasNodeName, "%s should have k8s.node.name on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasPodLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasPodLabel := false
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.pod.label.") {
						hasPodLabel = true
						break
					}
				}
				if hasPodLabel {
					break
				}
			}
			require.True(t, hasPodLabel, "%s should have k8s.pod.label.* on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			workloadCount := 0
			for _, r := range results {
				r := r
				if wn, ok := r.Labels.Resource["k8s.workload.name"]; ok && wn != "" {
					workloadCount++
					wt := r.Labels.Resource["k8s.workload.type"]
					require.True(t, wt != "", "%s has k8s.workload.name but empty k8s.workload.type", metricName)
				}
			}
			require.True(t, workloadCount > 0, "%s should have k8s.workload.* on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_HasSpecificWorkloadTypeAttr(t *testing.T) {
	t.Parallel()
	workloadAttrs := []string{
		"k8s.deployment.name", "k8s.statefulset.name", "k8s.daemonset.name",
		"k8s.replicaset.name", "k8s.job.name", "k8s.cronjob.name",
	}
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			hasSpecific := false
			for _, r := range results {
				r := r
				for _, attr := range workloadAttrs {
					attr := attr
					if val, ok := r.Labels.Resource[attr]; ok && val != "" {
						hasSpecific = true
						break
					}
				}
				if hasSpecific {
					break
				}
			}
			require.True(t, hasSpecific, "%s should have k8s.<workload>.name on at least some results", metricName)
		})
	}
}

func TestKSM_ContainerBucket_NginxDeploymentName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmContainerBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"nginx-test.*"}`,
				metricName, cfg.ClusterName)
			results, err := client.Query(ctx, promql)
			require.NoError(t, err, "querying %s for nginx-test", metricName)
			require.True(t, len(results) > 0, "No %s results from nginx-test pods", metricName)
			for _, r := range results {
				r := r
				require.Equal(t, "nginx-test", r.Labels.Resource["k8s.deployment.name"], "%s nginx-test container k8s.deployment.name", metricName)
				require.Equal(t, "nginx", r.Labels.Resource["k8s.container.name"], "%s nginx-test container k8s.container.name", metricName)
				require.Equal(t, "nginx-test", r.Labels.Resource["k8s.workload.name"], "%s nginx-test container k8s.workload.name", metricName)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "%s nginx-test container k8s.workload.type", metricName)
			}
		})
	}
}

// =============================================================================
// BUCKET 4: WORKLOAD METRICS
// =============================================================================

// --- Deployment ---

func TestKSM_WorkloadBucket_Deployment_HasDeploymentName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.deployment.name"]
				require.True(t, ok, "%s missing k8s.deployment.name", metricName)
				require.True(t, val != "", "%s has empty k8s.deployment.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing k8s.namespace.name", metricName)
				require.True(t, val != "", "%s has empty k8s.namespace.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				wn := r.Labels.Resource["k8s.workload.name"]
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wn != "", "%s missing k8s.workload.name", metricName)
				require.Equal(t, "Deployment", wt, "%s k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_NoCrossContamination(t *testing.T) {
	t.Parallel()
	wrongAttrs := []string{"k8s.statefulset.name", "k8s.daemonset.name", "k8s.job.name", "k8s.cronjob.name"}
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range wrongAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_HasRawDeploymentLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["deployment"]
				require.True(t, has, "%s missing raw 'deployment' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'deployment' label", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Deployment_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Deployment {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

// --- DaemonSet ---

func TestKSM_WorkloadBucket_DaemonSet_HasDaemonSetName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.daemonset.name"]
				require.True(t, ok, "%s missing k8s.daemonset.name", metricName)
				require.True(t, val != "", "%s has empty k8s.daemonset.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing k8s.namespace.name", metricName)
				require.True(t, val != "", "%s has empty k8s.namespace.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				wn := r.Labels.Resource["k8s.workload.name"]
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wn != "", "%s missing k8s.workload.name", metricName)
				require.Equal(t, "DaemonSet", wt, "%s k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_NoCrossContamination(t *testing.T) {
	t.Parallel()
	wrongAttrs := []string{"k8s.deployment.name", "k8s.statefulset.name", "k8s.job.name", "k8s.cronjob.name"}
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range wrongAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s (workloads span multiple nodes)", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_HasRawDaemonSetLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["daemonset"]
				require.True(t, has, "%s missing raw 'daemonset' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'daemonset' label", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_DaemonSet_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.DaemonSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

// --- StatefulSet workload tests ---

func TestKSM_WorkloadBucket_StatefulSet_HasName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			found := false
			for _, r := range results {
				r := r
				if r.Labels.Datapoint["statefulset"] == "ksm-test-statefulset" {
					found = true
				}
			}
			require.True(t, found, "%s should have ksm-test-statefulset", metricName)
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok && val != "", "%s missing k8s.namespace.name", metricName)
			}
		})
	}
}

// --- Job workload tests ---

func TestKSM_WorkloadBucket_Job_HasName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			found := false
			for _, r := range results {
				r := r
				if r.Labels.Datapoint["job_name"] == "ksm-test-job" {
					found = true
				}
			}
			require.True(t, found, "%s should have ksm-test-job", metricName)
		})
	}
}

func TestKSM_WorkloadBucket_Job_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok && val != "", "%s missing k8s.namespace.name", metricName)
			}
		})
	}
}

// --- CronJob workload tests ---

func TestKSM_WorkloadBucket_CronJob_HasName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			found := false
			for _, r := range results {
				r := r
				if r.Labels.Datapoint["cronjob"] == "ksm-test-cronjob" {
					found = true
				}
			}
			require.True(t, found, "%s should have ksm-test-cronjob", metricName)
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok && val != "", "%s missing k8s.namespace.name", metricName)
			}
		})
	}
}

// =============================================================================
// BUCKET 5: CLUSTER-SCOPED METRICS
// =============================================================================

func TestKSM_ClusterBucket_HasNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing k8s.namespace.name", metricName)
				require.True(t, val != "", "%s has empty k8s.namespace.name", metricName)
			}
		})
	}
}

func TestKSM_ClusterBucket_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_ClusterBucket_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_ClusterBucket_NoWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.workload.name"]
				require.True(t, !has, "%s should NOT have k8s.workload.name", metricName)
			}
		})
	}
}

func TestKSM_ClusterBucket_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

// =============================================================================
// LEASE VALIDATION
// =============================================================================

func TestKSM_LeaseExistence(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		restConfig, err = rest.InClusterConfig()
		require.NoError(t, err, "no kubeconfig or in-cluster config")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	require.NoError(t, err, "creating K8s clientset")

	ctx := context.Background()
	leases, err := clientset.CoordinationV1().Leases("amazon-cloudwatch").List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "listing Leases")

	leaseByNode := make(map[string]bool)
	for _, lease := range leases.Items {
		lease := lease
		if strings.HasPrefix(lease.Name, "cwagent-node-metadata-") {
			nodeName := strings.TrimPrefix(lease.Name, "cwagent-node-metadata-")
			leaseByNode[nodeName] = true

			annotations := lease.Annotations
			for _, key := range []string{
				"cwagent.amazonaws.com/host.id",
				"cwagent.amazonaws.com/host.name",
				"cwagent.amazonaws.com/host.type",
				"cwagent.amazonaws.com/host.image.id",
				"cwagent.amazonaws.com/cloud.availability_zone",
			} {
				key := key
				val, ok := annotations[key]
				require.True(t, ok, "Lease %s missing %s", lease.Name, key)
				require.True(t, val != "", "Lease %s has empty %s", lease.Name, key)
			}

			require.True(t, lease.Spec.LeaseDurationSeconds != nil, "Lease %s missing leaseDurationSeconds", lease.Name)
			require.Equal(t, int32(7200), *lease.Spec.LeaseDurationSeconds, "Lease %s leaseDurationSeconds", lease.Name)
		}
	}

	for nodeName := range gt.nodes {
		require.True(t, leaseByNode[nodeName], "node %s has no cwagent-node-metadata Lease", nodeName)
	}
}

// =============================================================================
// ADDITIONAL COMMON TESTS
// =============================================================================

func TestKSM_AllBuckets_ExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range allKSMMetricDefs {
		md := md
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		for _, label := range md.ExpectedLabels {
			label := label
			t.Run(md.Name+"/"+label, func(t *testing.T) {
				t.Parallel()
				results, err := queryCache.Get(context.Background(), md.Name)
				require.NoError(t, err, "querying %s", md.Name)
				require.NotEmpty(t, results, "%s not found", md.Name)
				for _, r := range results {
					r := r
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			})
		}
	}
}

func TestKSM_AllBuckets_UnitValidation(t *testing.T) {
	t.Parallel()
	for _, md := range allKSMMetricDefs {
		md := md
		if md.Unit == "" {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not found", md.Name)
			for _, r := range results {
				r := r
				unit, ok := r.Labels.Datapoint["__unit__"]
				require.True(t, ok, "%s missing __unit__", md.Name)
				require.Equal(t, md.Unit, unit, "%s unit", md.Name)
			}
		})
	}
}

func TestKSM_WorkloadBucket_HasRawNamespaceLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["namespace"]
				require.True(t, has, "%s missing raw 'namespace' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'namespace' label", metricName)
			}
		})
	}
}

func TestKSM_ClusterBucket_HasRawNamespaceLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmClusterBucket {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["namespace"]
				require.True(t, has, "%s missing raw 'namespace' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'namespace' label", metricName)
			}
		})
	}
}

func TestKSM_AllBuckets_NoPodTemplateHash(t *testing.T) {
	t.Parallel()
	for _, metricName := range allKSMBucketMetrics() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.pod.label.pod-template-hash"]
				require.True(t, !has, "%s should not have k8s.pod.label.pod-template-hash (removed by awsattributelimit)", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_HasCronJobName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.cronjob.name"]
				require.True(t, ok, "%s missing k8s.cronjob.name", metricName)
				require.True(t, val != "", "%s has empty k8s.cronjob.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_HasRawCronJobLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["cronjob"]
				require.True(t, has, "%s missing raw 'cronjob' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'cronjob' label", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				wn := r.Labels.Resource["k8s.workload.name"]
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wn != "", "%s missing k8s.workload.name", metricName)
				require.Equal(t, "CronJob", wt, "%s k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_NoCrossContamination(t *testing.T) {
	t.Parallel()
	wrongAttrs := []string{"k8s.deployment.name", "k8s.statefulset.name", "k8s.daemonset.name", "k8s.job.name"}
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range wrongAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_CronJob_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.CronJob {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_HasJobName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.job.name"]
				require.True(t, ok, "%s missing k8s.job.name", metricName)
				require.True(t, val != "", "%s has empty k8s.job.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_HasRawJobNameLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["job_name"]
				require.True(t, has, "%s missing raw 'job_name' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'job_name' label", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				wn := r.Labels.Resource["k8s.workload.name"]
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wn != "", "%s missing k8s.workload.name", metricName)
				require.Equal(t, "Job", wt, "%s k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_NoCrossContamination(t *testing.T) {
	t.Parallel()
	wrongAttrs := []string{"k8s.deployment.name", "k8s.statefulset.name", "k8s.daemonset.name", "k8s.cronjob.name"}
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range wrongAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_Job_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.Job {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_HasRawStatefulSetLabel(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, has := r.Labels.Datapoint["statefulset"]
				require.True(t, has, "%s missing raw 'statefulset' label at datapoint scope", metricName)
				require.True(t, val != "", "%s has empty raw 'statefulset' label", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_HasStatefulSetName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				val, ok := r.Labels.Resource["k8s.statefulset.name"]
				require.True(t, ok, "%s missing k8s.statefulset.name", metricName)
				require.True(t, val != "", "%s has empty k8s.statefulset.name", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_HasWorkloadIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not found", metricName)
			for _, r := range results {
				r := r
				wn := r.Labels.Resource["k8s.workload.name"]
				wt := r.Labels.Resource["k8s.workload.type"]
				require.True(t, wn != "", "%s missing k8s.workload.name", metricName)
				require.Equal(t, "StatefulSet", wt, "%s k8s.workload.type", metricName)
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_NoCrossContamination(t *testing.T) {
	t.Parallel()
	wrongAttrs := []string{"k8s.deployment.name", "k8s.daemonset.name", "k8s.job.name", "k8s.cronjob.name"}
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range wrongAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_NoHostAttributes(t *testing.T) {
	t.Parallel()
	hostAttrs := []string{"host.id", "host.name", "host.type", "host.image.id"}
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hostAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should NOT have %s", metricName, attr)
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_NoNodeLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.HasPrefix(key, "k8s.node.label.") {
						t.Fatalf("%s should NOT have node labels, found %s", metricName, key)
					}
				}
			}
		})
	}
}

func TestKSM_WorkloadBucket_StatefulSet_NoNodeName(t *testing.T) {
	t.Parallel()
	for _, metricName := range ksmWorkloadBucket.StatefulSet {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Resource["k8s.node.name"]
				require.True(t, !has, "%s should NOT have k8s.node.name", metricName)
			}
		})
	}
}
