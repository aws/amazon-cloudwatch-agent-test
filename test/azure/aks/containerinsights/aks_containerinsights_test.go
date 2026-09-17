// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package containerinsights validates that the CloudWatch Agent translates the OTEL Container
// Insights JSON config on AKS and delivers metrics/logs to CloudWatch over the AKS
// workload-identity -> STS web-identity chain. Terraform mounts ci_node.json (role=node,
// logs enabled) and ci_cluster.json (role=cluster, keda+karpenter) into the agent workloads
// (no USE_DEFAULT_CONFIG), so the agent's own translator builds the pipelines.
package containerinsights

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// validationWindow bounds every CloudWatch lookup (scrape + export + ingestion latency).
const validationWindow = 10 * time.Minute

var env *environment.MetaData

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	// AKSClusterName is the k8s.cluster.name stamped on all telemetry; scopes this run.
	if env.AKSClusterName == "" {
		fmt.Fprintln(os.Stderr, "aksClusterName flag is required to scope telemetry to this cluster")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// role=node: cadvisor / kubeletstats / node_exporter.
var nodeMetrics = []string{
	"container_cpu_usage_seconds_total",
	"container_memory_working_set_bytes",
	"k8s.node.cpu.usage",
	"k8s.node.memory.working_set",
	"k8s.pod.cpu.usage",
	"node_cpu_seconds_total",
	"node_memory_MemAvailable_bytes",
}

// role=cluster: apiserver + kube-state-metrics.
var clusterMetrics = []string{
	"apiserver_request_total",
	"kube_node_info",
	"kube_pod_info",
}

// role=cluster keda/karpenter solution pipelines (scraped from the stub emitters).
var kedaMetrics = []string{"keda_scaler_active", "keda_scaledobject_paused"}
var karpenterMetrics = []string{"karpenter_nodes_total", "karpenter_pods_state"}

func TestAKSContainerInsights(t *testing.T) {
	deadline := time.Now().Add(validationWindow)

	t.Run("NodeMetrics", func(t *testing.T) { validateMetrics(t, nodeMetrics, deadline) })
	t.Run("ClusterMetrics", func(t *testing.T) { validateMetrics(t, clusterMetrics, deadline) })
	t.Run("KedaMetrics", func(t *testing.T) { validateMetrics(t, kedaMetrics, deadline) })
	t.Run("KarpenterMetrics", func(t *testing.T) { validateMetrics(t, karpenterMetrics, deadline) })
	t.Run("NodeLogs", testNodeApplicationLogs)
}

// validateMetrics asserts each metric is present for this cluster. cloud.platform=azure_aks
// proves the agent ran the RUN_IN_AKS translation path, not a hardcoded EKS/EC2 one.
func validateMetrics(t *testing.T, metrics []string, deadline time.Time) {
	labels := map[string]string{
		"@resource.k8s.cluster.name": env.AKSClusterName,
		"@resource.cloud.platform":   "azure_aks",
	}

	// ValidateOtlpMetricsWithLabels retries internally (~90s); wrap it in a bounded poll so a
	// slow-to-propagate category keeps checking until the shared deadline instead of failing early.
	const pollInterval = 15 * time.Second
	allSuccessful := func(g status.TestGroupResult) bool {
		if len(g.TestResults) == 0 {
			return false
		}
		for _, r := range g.TestResults {
			if r.Status != status.SUCCESSFUL {
				return false
			}
		}
		return true
	}

	var group status.TestGroupResult
	for {
		group = otlpvalidation.ValidateOtlpMetricsWithLabels(t.Name(), env.Region, metrics, labels)
		if allSuccessful(group) || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	for _, r := range group.TestResults {
		r := r
		t.Run(r.Name, func(t *testing.T) {
			require.Equal(t, status.SUCCESSFUL, r.Status,
				"metric %s (cluster=%s): %v", r.Name, env.AKSClusterName, r.Reason)
		})
	}
}

// testNodeApplicationLogs asserts the role=node logs pipeline delivered application logs.
// Cleans up the group only on success; on failure it is left as debugging evidence.
func testNodeApplicationLogs(t *testing.T) {
	logGroup := fmt.Sprintf("/aws/otel/containerinsights/%s/application", env.AKSClusterName)

	succeeded := false
	defer func() {
		if succeeded {
			awsservice.DeleteLogGroup(logGroup)
		}
	}()

	const maxRetries = 4
	const retryInterval = 30 * time.Second
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		streams := awsservice.GetLogStreamNames(logGroup)
		if len(streams) > 0 {
			since := time.Now().Add(-validationWindow)
			until := time.Now()
			lastErr = awsservice.ValidateLogs(logGroup, streams[0], &since, &until, awsservice.AssertLogsNotEmpty())
			if lastErr == nil {
				succeeded = true
				return
			}
		} else {
			lastErr = fmt.Errorf("no log streams in %s yet", logGroup)
		}
		log.Printf("[AKS_CI_Logs] attempt %d: %v", attempt, lastErr)
		if attempt < maxRetries {
			time.Sleep(retryInterval)
		}
	}
	require.NoError(t, lastErr, "validating application logs in %s", logGroup)
}
