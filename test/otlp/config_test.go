// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlp

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

const (
	agentRuntime          = 5 * time.Minute
	loadGeneratorInterval = 5 * time.Second
	testSegmentCount      = 20
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestTraces(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testCases := map[string]struct {
		agentConfigPath string
		generatorConfig *base.TraceGeneratorConfig
	}{
		"WithOTLP/Simple": {
			agentConfigPath: filepath.Join("resources", "otlp-config.json"),
			generatorConfig: &base.TraceGeneratorConfig{
				Interval: loadGeneratorInterval,
				Annotations: map[string]interface{}{
					"test_type":   "simple_otlp",
					"instance_id": env.InstanceId,
					"commit_sha":  env.CwaCommitSha,
				},
				Metadata: map[string]map[string]interface{}{
					"default": {
						"custom_key": "custom_value",
					},
				},
				Attributes: []attribute.KeyValue{
					attribute.String("custom_key", "custom_value"),
					attribute.String("test_type", "simple_otlp"),
					attribute.String("instance_id", env.InstanceId),
					attribute.String("commit_sha", env.CwaCommitSha),
				},
			},
		},
		"WithOTLP/Enhanced": {
			agentConfigPath: filepath.Join("resources", "otlp-traces-config.json"),
			generatorConfig: &base.TraceGeneratorConfig{
				Interval: loadGeneratorInterval,
				Annotations: map[string]interface{}{
					"test_type":     "enhanced_otlp",
					"instance_id":   env.InstanceId,
					"commit_sha":    env.CwaCommitSha,
					"service_name":  "otlp-trace-test",
					"operation":     "test_operation",
					"trace_version": "v2.0",
				},
				Metadata: map[string]map[string]interface{}{
					"default": {
						"custom_key":    "custom_value",
						"test_metadata": "enhanced_test",
						"environment":   "integration_test",
					},
					"http": {
						"method": "POST",
						"url":    "/api/test",
						"status": 200,
					},
				},
				Attributes: []attribute.KeyValue{
					attribute.String("custom_key", "custom_value"),
					attribute.String("test_type", "enhanced_otlp"),
					attribute.String("instance_id", env.InstanceId),
					attribute.String("commit_sha", env.CwaCommitSha),
					attribute.String("service.name", "otlp-trace-test"),
					attribute.String("service.version", "1.0.0"),
					attribute.String("http.method", "POST"),
					attribute.String("http.url", "/api/test"),
					attribute.Int("http.status_code", 200),
					attribute.Bool("test.enabled", true),
				},
			},
		},
	}
	t.Logf("Sanity check: number of test cases:%d", len(testCases))
	for name, testCase := range testCases {

		t.Run(name, func(t *testing.T) {

			OtlpTestCfg := base.TraceTestConfig{
				Generator:       otlp.NewLoadGenerator(testCase.generatorConfig),
				Name:            name,
				AgentConfigPath: testCase.agentConfigPath,
				AgentRuntime:    agentRuntime,
			}
			err := base.TraceTest(t, OtlpTestCfg)
			require.NoError(t, err, "TraceTest failed because %s", err)

		})
	}
}
