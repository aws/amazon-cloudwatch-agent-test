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
  resource/profiles:
    attributes:
      - action: insert
        key: service.name
        value: unknown_service
  transform/identity:
    profile_statements: []
service:
  pipelines:
    metrics/otlp:
      receivers: [otlp/grpc_127_0_0_1_4317]
      processors: [batch/hostOtlpMetrics/cloudwatchlogs]
      exporters: [awsemf]
    profiles/otlp:
      receivers: [otlp/grpc_127_0_0_1_4317, otlp/http_127_0_0_1_4318]
      processors: [resourcedetection/opentelemetry, transform/identity, resource/profiles]
      exporters: [otlp_http/profiles]
`

func TestProfilesPipelineInvariantsPass(t *testing.T) {
	p, err := parseProfilesPipeline([]byte(goodTranslatedConfig))
	require.NoError(t, err)
	// A batch processor in another pipeline must not trip the check.
	require.NoError(t, checkProfilesPipelineInvariants(p))
	require.Equal(t, []string{fallbackServiceName}, serviceNameInserts(p.resourceActions))
}

func TestProfilesPipelineInvariantsBatch(t *testing.T) {
	p := profilesPipeline{pipelineConfig: pipelineConfig{Processors: []string{"batch/profiles", "resource/profiles"}},
		resourceActions: []attributeAction{{Action: "insert", Key: serviceNameKey, Value: fallbackServiceName}}}
	err := checkProfilesPipelineInvariants(p)
	require.ErrorContains(t, err, "batch/profiles")

	p.Processors = []string{"batch", "resource/profiles"}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "processor batch is in")
}

func TestProfilesPipelineInvariantsServiceName(t *testing.T) {
	p := profilesPipeline{pipelineConfig: pipelineConfig{Processors: []string{"transform/identity"}}}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "resource/profiles is missing")

	p.Processors = []string{"resource/profiles"}
	p.resourceActions = []attributeAction{{Action: "delete", Key: serviceNameKey}}
	require.ErrorContains(t, checkProfilesPipelineInvariants(p), "no insert or upsert action")
}

func TestParseProfilesPipelineMissing(t *testing.T) {
	_, err := parseProfilesPipeline([]byte("service:\n  pipelines:\n    metrics/otlp: {}\n"))
	require.ErrorContains(t, err, "no profiles/otlp pipeline")

	_, err = parseProfilesPipeline([]byte("service: [unclosed"))
	require.ErrorContains(t, err, "parsing translated config")
}

func TestServiceNameInsertsOrder(t *testing.T) {
	actions := []attributeAction{
		{Action: "insert", Key: serviceNameKey, Value: "inferred-svc"},
		{Action: "insert", Key: "other", Value: "x"},
		{Action: "insert", Key: serviceNameKey, Value: fallbackServiceName},
	}
	require.Equal(t, []string{"inferred-svc", fallbackServiceName}, serviceNameInserts(actions))
}
