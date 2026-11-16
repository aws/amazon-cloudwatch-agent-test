//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
)

// TestProfiles checks the chart's profiles/cw_k8s_otlp pipeline when the suite installs the chart with
// otlp.profiles.enabled=true (the otlp_profiles_enabled Terraform variable, which sets
// OTLP_PROFILES_ENABLED): the node agent's otelConfig has the pipeline, the app sender's profiles posts
// through the Service are accepted, and no agent logs a failed or rejected export from the profiles
// exporter. There is no CloudWatch read API for profiles to check delivery against, so like the EC2
// profiles test (test/otel_collect/linux/profiles) it checks the agent's side of the export.
//
// The flag is off by default because otlp.profiles.enabled is off in the chart and the pipeline needs
// an agent image with the collector's profiles support (1.300075.0 or later).
func TestProfiles(t *testing.T) {
	if !otlpendpoint.ProfilesEnabled() {
		t.Skipf("%s is not true: the chart is installed without otlp.profiles.enabled", otlpendpoint.ProfilesEnabledEnv)
	}
	ctx := context.Background()

	otelConfig, err := otlpendpoint.NodeAgentOtelConfig(ctx, kube)
	require.NoError(t, err)
	exporters, found, err := otlpendpoint.PipelineExporters(otelConfig, otlpendpoint.ProfilesPipeline)
	require.NoError(t, err)
	require.True(t, found, "the node agent's otelConfig has no %s pipeline", otlpendpoint.ProfilesPipeline)
	assert.Equal(t, []string{otlpendpoint.ProfilesExporter}, exporters, "%s exporters", otlpendpoint.ProfilesPipeline)

	const minAccepted = 3
	for _, pod := range appPods(t) {
		statuses, err := otlpendpoint.SendStatuses(ctx, kube, senderNamespace, pod.Name)
		require.NoError(t, err)
		posts := statuses[otlpendpoint.ProfilesSignal]
		require.NotEmpty(t, posts, "%s: no profiles posts in the sender log", pod.Name)
		accepted := 0
		for _, status := range posts {
			if status == "200" {
				accepted++
			}
		}
		assert.GreaterOrEqual(t, accepted, minAccepted, "%s: accepted profiles posts of %v", pod.Name, posts)
		assert.Equal(t, "200", posts[len(posts)-1], "%s: last profiles post", pod.Name)
	}

	// The exporter batches and retries, so give the posts above time to be exported before reading the
	// agent logs.
	time.Sleep(time.Minute)
	logs, err := otlpendpoint.AgentLogs(ctx, kube)
	require.NoError(t, err)
	require.NotEmpty(t, logs, "no running node agent pods")
	for container, log := range logs {
		assert.Empty(t, otlpendpoint.ExporterProblems(log, otlpendpoint.ProfilesExporter), "%s: %s export problems", container, otlpendpoint.ProfilesExporter)
	}
}
