//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package otlp_disabled tests the Helm chart with otlp.enabled=false next to OTel Container Insights
// (terraform/eks/daemon/otel-otlp-disabled): the node agent serves no OTLP endpoint, so a pod that
// sends OTLP through the cloudwatch-agent Service fails and nothing reaches CloudWatch.
package otlp_disabled

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
)

const (
	probeNamespace = "otlp-e2e"
	probeSelector  = "app=otlp-e2e-probe"

	// minPosts is the number of posts per signal the probe must have made before its failures count.
	minPosts = 3

	attempts      = 10
	retryInterval = 30 * time.Second
)

// TestNoOTLPPorts checks that the operator opened neither default OTLP port on the node agent's
// Service or host network. The operator creates the Service only when the agent's configuration
// yields a port, so a missing Service also means no OTLP port. TestEndpoint in test/otel/otlp requires
// a Service of the same name, and AgentHostPorts fails if the DaemonSet is missing.
func TestNoOTLPPorts(t *testing.T) {
	ctx := context.Background()
	servicePorts, found, err := otlpendpoint.ServicePorts(ctx, kube)
	require.NoError(t, err)
	if !found {
		t.Logf("no %s/%s Service: the agent's configuration opens no port", otlpendpoint.AgentNamespace, otlpendpoint.AgentName)
	}
	hostPorts, err := otlpendpoint.AgentHostPorts(ctx, kube)
	require.NoError(t, err)
	for _, port := range []int32{otlpendpoint.GRPCPort, otlpendpoint.HTTPPort} {
		assert.NotContains(t, servicePorts, port, "Service port")
		assert.NotContains(t, hostPorts, port, "agent host port")
	}
}

// TestSendFails checks that no post from the probe pod through the Service succeeded.
func TestSendFails(t *testing.T) {
	ctx := context.Background()
	pods, err := otlpendpoint.RunningPods(ctx, kube, probeNamespace, probeSelector)
	require.NoError(t, err)
	require.NotEmpty(t, pods, "no running probe pod")
	for _, pod := range pods {
		statuses, err := otlpendpoint.SendStatuses(ctx, kube, probeNamespace, pod.Name)
		require.NoError(t, err)
		for _, signal := range otlpendpoint.Signals {
			require.GreaterOrEqual(t, len(statuses[signal]), minPosts, "%s: %s posts in the probe log", pod.Name, signal)
			for _, status := range statuses[signal] {
				assert.Equal(t, "000", status, "%s: %s post reached a server", pod.Name, signal)
			}
		}
	}
}

// TestNoOTLPTelemetry checks that no metric with the OTLP pipelines' scope arrived for this cluster.
// It runs only once OTel Container Insights metrics from the node agents are seen, so that it cannot
// pass because the agents export nothing.
func TestNoOTLPTelemetry(t *testing.T) {
	ctx := context.Background()
	ci := fmt.Sprintf(`{"__name__"="container_cpu_usage_seconds_total","@resource.k8s.cluster.name"=%q}`, cfg.ClusterName)
	for i := 0; ; i++ {
		results, err := client.Query(ctx, ci)
		require.NoError(t, err)
		if len(results) > 0 {
			break
		}
		require.Less(t, i, attempts, "no OTel Container Insights metrics for the cluster")
		time.Sleep(retryInterval)
	}

	matcher := fmt.Sprintf(`{__name__=~".+","@resource.k8s.cluster.name"=%q,"@instrumentation.cloudwatch.solution"=%q}`,
		cfg.ClusterName, otlpendpoint.Solution)
	names, err := client.MetricNames(ctx, matcher, time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	assert.Empty(t, names, "metrics with the OTLP scope")
}
