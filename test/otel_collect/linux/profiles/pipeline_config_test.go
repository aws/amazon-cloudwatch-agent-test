// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const goodTranslatedConfig = `
processors:
  batch/hostOtlpMetrics/cloudwatchlogs:
    timeout: 30s
  transform/identity:
    profile_statements:
      - context: resource
        statements:
          - set(resource.attributes["service.name"], "unknown_service") where resource.attributes["service.name"] == nil
service:
  pipelines:
    metrics/otlp:
      receivers: [otlp/grpc_127_0_0_1_4317]
      processors: [batch/hostOtlpMetrics/cloudwatchlogs]
      exporters: [awsemf]
    profiles/otlp:
      receivers: [otlp/grpc_127_0_0_1_4317, otlp/http_127_0_0_1_4318]
      processors: [resourcedetection/opentelemetry, transform/identity]
      exporters: [otlp_http/profiles]
`

func TestProfilesPipelineInvariantsPass(t *testing.T) {
	p, err := parseProfilesPipeline([]byte(goodTranslatedConfig))
	require.NoError(t, err)
	require.NoError(t, checkProfilesPipelineInvariants(p))
	require.True(t, hasFallbackServiceName(p.identityProfileStatements))
}

func TestProfilesPipelineInvariantsBatch(t *testing.T) {
	fallback := []string{`set(resource.attributes["service.name"], "unknown_service") where resource.attributes["service.name"] == nil`}
	p := profilesPipeline{pipelineConfig: pipelineConfig{Processors: []string{"batch/profiles", "transform/identity"}},
		identityProfileStatements: fallback}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "batch/profiles")

	p.Processors = []string{"batch", "transform/identity"}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "processor batch is in")
}

func TestProfilesPipelineInvariantsServiceName(t *testing.T) {
	p := profilesPipeline{pipelineConfig: pipelineConfig{Processors: []string{"transform/identity"}}}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "sets no service.name")
}

func TestProfilesPipelineInvariantsIdentityFallback(t *testing.T) {
	p := profilesPipeline{pipelineConfig: pipelineConfig{Processors: []string{"k8s_attributes/profiles", "transform/identity"}},
		identityProfileStatements: []string{
			`set(resource.attributes["k8s.workload.name"], attributes["k8s.deployment.name"]) where resource.attributes["k8s.workload.name"] == nil`,
			`set(resource.attributes["service.name"], "unknown_service") where resource.attributes["service.name"] == nil`,
		}}
	require.NoError(t, checkProfilesPipelineInvariants(p))

	p.Processors = append(p.Processors, "batch")
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "processor batch is in")
}

func TestHasFallbackServiceName(t *testing.T) {
	require.True(t, hasFallbackServiceName([]string{
		`set(resource.attributes["service.name"], "unknown_service") where resource.attributes["service.name"] == nil`,
	}))
	require.False(t, hasFallbackServiceName([]string{
		`set(resource.attributes["cloud.resource_id"], resource.attributes["aws.ecs.task.arn"]) where resource.attributes["cloud.resource_id"] == nil and resource.attributes["aws.ecs.task.arn"] != nil`,
		`set(resource.attributes["aws.log.stream.name"], Concat([resource.attributes["host.id"], resource.attributes["service.name"]], "/")) where resource.attributes["service.name"] != "unknown_service"`,
	}))
	require.False(t, hasFallbackServiceName(nil))
}

func TestParseProfilesPipelineMissing(t *testing.T) {
	_, err := parseProfilesPipeline([]byte("service:\n  pipelines:\n    metrics/otlp: {}\n"))
	require.ErrorContains(t, err, "no profiles/otlp pipeline")

	_, err = parseProfilesPipeline([]byte("service: [unclosed"))
	require.ErrorContains(t, err, "parsing translated config")
}

const kubernetesTranslatedConfig = `
processors:
  transform/identity:
    profile_statements:
      - context: resource
        statements:
          - set(resource.attributes["k8s.workload.name"], resource.attributes["k8s.deployment.name"]) where resource.attributes["k8s.workload.name"] == nil and resource.attributes["k8s.deployment.name"] != nil
          - set(resource.attributes["service.name"], "unknown_service") where resource.attributes["service.name"] == nil
service:
  pipelines:
    profiles/otlp:
      receivers: [otlp/grpc_127_0_0_1_4317]
      processors: [resourcedetection/opentelemetry, k8s_attributes/profiles, transform/set_cluster_name, transform/identity]
      exporters: [otlp_http/profiles]
`

func TestProfilesPipelineInvariantsKubernetes(t *testing.T) {
	p, err := parseProfilesPipeline([]byte(kubernetesTranslatedConfig))
	require.NoError(t, err)
	require.True(t, hasFallbackServiceName(p.identityProfileStatements))
	require.NoError(t, checkProfilesPipelineInvariants(p))
}
