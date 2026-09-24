// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package aks validates the agent on a real AKS cluster installed via the amazon-cloudwatch-observability
// Helm chart (the scripts/azure/setup.sh onboarding path): a DaemonSet agent running default:otel plus a
// cluster-scraper Deployment, with Container Insights on. A load-generator Job pushes OTLP to the DaemonSet
// agent via hostNetwork, and this test validates OTLP, spanmetrics, Container Insights metrics, logs, and
// traces reach CloudWatch via the AKS projected-token → AWS STS web-identity federation chain.
package aks

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	spansLogGroup = "aws/spans"
	serviceName   = "aks-otlp-test-service"
	// agentNamespace is the k8s namespace the agent runs in. The resource k8s.namespace.name and the
	// derived service.namespace both carry it.
	agentNamespace = "amazon-cloudwatch"
	// The load generator runs for 3 minutes, so allow extra ingestion time.
	validationWindow = 10 * time.Minute
)

var env *environment.MetaData

// otlpMetrics are the metrics the load generator pushes over OTLP.
var otlpMetrics = []string{
	"aks_otlp_counter",
}

// spanMetrics are the spanmetrics connector's metrics, derived from the pushed spans. Validated on
// @resource.* labels alone because the connector scope carries no cloudwatch.source/solution.
var spanMetrics = []string{
	"traces.span.metrics.calls",
	"traces.span.metrics.duration",
}

// containerInsightsMetrics are Container Insights metrics the helm chart's otelContainerInsights pipeline
// emits: kubeletstats and cadvisor from the DaemonSet agent, kube-state-metrics from the cluster scraper.
// They carry the kubeletstats/prometheus instrumentation scope rather than the cloudwatch OTLP scope, so
// they are validated on @resource.k8s.cluster.name alone, like spanMetrics. This is the high-confidence
// subset shared with the EKS suite (test/otel/standard); EKS control-plane metrics (apiserver_*) are
// omitted because AKS runs a managed control plane the agent does not scrape.
var containerInsightsMetrics = []string{
	// kubeletstats: node, pod, and container scoped.
	"k8s.node.cpu.usage",
	"k8s.node.memory.working_set",
	"k8s.node.filesystem.available",
	"k8s.pod.cpu.usage",
	"k8s.pod.memory.working_set",
	"k8s.pod.network.io",
	"container.cpu.usage",
	"container.memory.working_set",
	"container.memory.usage",
	// cadvisor.
	"container_cpu_usage_seconds_total",
	"container_memory_working_set_bytes",
	"container_memory_usage_bytes",
	"container_network_receive_bytes_total",
	// kube-state-metrics, from the cluster scraper deployment.
	"kube_node_status_condition",
	"kube_node_status_allocatable",
	"kube_node_status_capacity",
	"kube_pod_status_phase",
	"kube_pod_container_status_running",
	"kube_deployment_status_replicas",
	"kube_deployment_status_replicas_ready",
	"kube_daemonset_status_desired_number_scheduled",
	"kube_namespace_status_phase",
}

// aksResourceExpectations is the shared source of truth for the azure.aks resourcedetection attributes
// asserted on every signal: an exact value, or PresenceOnly for present-and-non-empty. Node-level and
// subscription-bearing attributes stay presence-only (their values are per-node/dynamic, or must be kept
// out of the CI logs).
func aksResourceExpectations() map[string]string {
	return map[string]string{
		"cloud.provider":              "azure",
		"cloud.platform":              "azure.aks",
		"k8s.cluster.name":            env.AKSClusterName,
		"k8s.namespace.name":          agentNamespace,
		"service.namespace":           agentNamespace,
		"deployment.environment.name": "azure.aks:" + env.AKSClusterName + "/" + agentNamespace,
		"cloud.region":                env.AzureLocation,
		"azure.vm.size":               env.AzureVMSize,        // AKS node pool VM size
		"azure.resourcegroup.name":    env.AzureResourceGroup, // AKS node resource group (MC_...)
		"cloud.account.id":            otlpvalidation.PresenceOnly,
		"cloud.resource_id":           otlpvalidation.PresenceOnly,
		// Per-node VMSS-generated names / machine id: not knowable at plan time.
		"host.id":                otlpvalidation.PresenceOnly,
		"host.name":              otlpvalidation.PresenceOnly,
		"azure.vm.name":          otlpvalidation.PresenceOnly,
		"azure.vm.scaleset.name": otlpvalidation.PresenceOnly,
		"service.name":           otlpvalidation.PresenceOnly,
	}
}

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	if env.AKSClusterName == "" {
		fmt.Fprintln(os.Stderr, "aksClusterName flag is required to scope telemetry to this cluster")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestAKS(t *testing.T) {
	t.Run("Metrics", func(t *testing.T) {
		// Isolated by @resource.k8s.cluster.name, which is unique per run and never reused. Agent-produced
		// metrics carry the cloudwatch instrumentation scope. Spanmetrics carry only the @resource.*
		// enrichment, so they match on resource labels alone.
		resourceLabels := map[string]string{}
		for attr, want := range aksResourceExpectations() {
			resourceLabels["@resource."+attr] = otlpvalidation.ExpectedValue(want)
		}
		fullLabels := map[string]string{}
		for k, v := range resourceLabels {
			fullLabels[k] = v
		}
		for attr, want := range otlpvalidation.ScopeExpectations() {
			fullLabels["@instrumentation."+attr] = otlpvalidation.ExpectedValue(want)
		}
		// Container Insights and spanmetrics span multiple nodes and carry their own instrumentation
		// scopes, so they are matched on the cluster attribute alone.
		clusterLabels := map[string]string{"@resource.k8s.cluster.name": env.AKSClusterName}
		assertFound := func(t *testing.T, group status.TestGroupResult) {
			for _, r := range group.TestResults {
				require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s: %v", r.Name, r.Reason)
			}
		}

		t.Run("OTLP", func(t *testing.T) {
			assertFound(t, otlpvalidation.ValidateOtlpMetricsWithLabels("AKSOTLP", env.Region, otlpMetrics, fullLabels))
		})
		t.Run("SpanMetrics", func(t *testing.T) {
			assertFound(t, otlpvalidation.ValidateOtlpMetricsWithLabels("AKSSpanMetrics", env.Region, spanMetrics, resourceLabels))
		})
		t.Run("ContainerInsights", func(t *testing.T) {
			assertFound(t, otlpvalidation.ValidateOtlpMetricsWithLabels("AKSContainerInsights", env.Region, containerInsightsMetrics, clusterLabels))
		})
		t.Run("Names", func(t *testing.T) {
			// Enumerate every metric name produced for this cluster and assert the set the subtests above
			// cover is present. Catches drift where an expected metric stops emitting. This is a subset
			// rather than an exact match: the Container Insights pipeline emits a broad, environment-
			// dependent set (per-container cadvisor series, per-object kube-state-metrics), so pinning the
			// full list exactly would be brittle.
			matcher := fmt.Sprintf(`{__name__=~".+", "@resource.k8s.cluster.name"=%q}`, env.AKSClusterName)
			got, err := otlpvalidation.MetricNamesForMatcher(env.Region, matcher, time.Now().Add(-time.Hour), time.Now())
			require.NoError(t, err)
			want := append(append(append([]string{}, otlpMetrics...), spanMetrics...), containerInsightsMetrics...)

			// Log the metrics produced but not yet in our expected set, so we can see what Container
			// Insights emits on AKS and grow containerInsightsMetrics toward an eventual exact match.
			wantSet := make(map[string]bool, len(want))
			for _, m := range want {
				wantSet[m] = true
			}
			var extra []string
			for _, m := range got {
				if !wantSet[m] {
					extra = append(extra, m)
				}
			}
			sort.Strings(extra)
			t.Logf("%d metric names produced for cluster; %d not in expected set:\n%s",
				len(got), len(extra), strings.Join(extra, "\n"))

			require.Subset(t, got, want)
		})
	})

	t.Run("Logs", func(t *testing.T) {
		r := validateLogs()
		require.Equal(t, status.SUCCESSFUL, r.Status, "log validation failed: %v", r.Reason)
	})

	t.Run("Traces", func(t *testing.T) {
		r := validateTraces()
		require.Equal(t, status.SUCCESSFUL, r.Status, "trace validation failed: %v", r.Reason)
	})
}

func validateLogs() status.TestResult {
	testResult := status.TestResult{Name: "AKS_Logs", Status: status.FAILED}

	// The agent's k8s logs routing derives the destination from the k8s.cluster.name and
	// k8s.namespace.name resource attributes the load generator sends, so it is unique to
	// this cluster. The stream is {k8s.namespace.name}/{service.namespace}/{service.name},
	// where the agent's identity transform fills service.namespace from k8s.namespace.name.
	// AssertLogsNotEmpty guards against a vacuous pass on an empty window.
	logGroup := fmt.Sprintf("/aws/cwagent/%s/otlp", env.AKSClusterName)
	// Clean up only on success: the group name carries this run's cluster so the whole group is
	// disposable, but on failure it is left in place as evidence for whoever debugs the run.
	defer func() {
		if testResult.Status == status.SUCCESSFUL {
			awsservice.DeleteLogGroup(logGroup)
		}
	}()
	logStream := fmt.Sprintf("amazon-cloudwatch/amazon-cloudwatch/%s", serviceName)
	marker := fmt.Sprintf("aks_otlp_log_%s", env.AKSClusterName)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		until := time.Now()
		log.Printf("[AKS_Logs] attempt %d: checking %s/%s", attempt, logGroup, logStream)
		err := awsservice.ValidateLogs(
			logGroup, logStream, &since, &until,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(otlpvalidation.AssertLogRecord(func(rec otlpvalidation.LogRecord) error {
				return otlpvalidation.AssertLogContent(rec, marker, "INFO", aksResourceExpectations())
			})),
		)
		if err == nil {
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		testResult.Reason = err
		if attempt < maxRetries {
			log.Printf("[AKS_Logs] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// validateTraces queries aws/spans (Transaction Search) for this run's spans. The load generator is an
// external k8s Job, so we cannot enumerate trace IDs. Match spans by service name + cluster instead, then
// assert the azure.aks resource enrichment on each matched span (not just that some arrived).
func validateTraces() status.TestResult {
	testResult := status.TestResult{Name: "AKS_Traces", Status: status.FAILED}

	query := fmt.Sprintf(
		`fields @message | filter @message like "%s" and @message like "%s" | limit 100`,
		serviceName, env.AKSClusterName,
	)
	log.Printf("[AKS_Traces] querying %s for spans from service=%s cluster=%s", spansLogGroup, serviceName, env.AKSClusterName)

	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		rows, err := awsservice.GetLogQueryResults(spansLogGroup, since.Unix(), time.Now().Unix(), query)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: %s query failed: %w", attempt, spansLogGroup, err)
		} else {
			found := 0
			for _, row := range rows {
				for _, field := range row {
					if aws.ToString(field.Field) != "@message" {
						continue
					}
					var s otlpvalidation.SpanRecord
					if json.Unmarshal([]byte(aws.ToString(field.Value)), &s) != nil {
						continue
					}
					// Content is stable, so a mismatch is final. Fail immediately.
					if cerr := otlpvalidation.AssertAttributes("span "+s.TraceID, aksResourceExpectations(), s.Resource.Attributes); cerr != nil {
						testResult.Reason = cerr
						return testResult
					}
					found++
				}
			}
			if found > 0 {
				log.Printf("[AKS_Traces] attempt %d: %d spans found with expected content in %s", attempt, found, spansLogGroup)
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: 0 spans found in %s for service=%s", attempt, spansLogGroup, serviceName)
		}
		if attempt < maxRetries {
			log.Printf("[AKS_Traces] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}
