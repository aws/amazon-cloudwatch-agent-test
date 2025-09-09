// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

const (
	testRuntime           = 4 * time.Minute
	namespace             = "CWAgent/OTLP"
	logGroup              = "/aws/amazon-cloudwatch-agent/otlp"
	agentRuntime          = 5 * time.Minute
	loadGeneratorInterval = 5 * time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

//func TestTraces(t *testing.T) {
//	env := environment.GetEnvironmentMetaData()
//	testCases := map[string]struct {
//		agentConfigPath string
//		generatorConfig *base.TraceGeneratorConfig
//	}{
//		"WithOTLP/Simple": {
//			agentConfigPath: filepath.Join("agent_configs", "otlp-config.json"),
//			generatorConfig: &base.TraceGeneratorConfig{
//				Interval: loadGeneratorInterval,
//				Annotations: map[string]interface{}{
//					"test_type":   "simple_otlp",
//					"instance_id": env.InstanceId,
//					"commit_sha":  env.CwaCommitSha,
//				},
//				Metadata: map[string]map[string]interface{}{
//					"default": {
//						"custom_key": "custom_value",
//					},
//				},
//				Attributes: []attribute.KeyValue{
//					attribute.String("custom_key", "custom_value"),
//					attribute.String("test_type", "simple_otlp"),
//					attribute.String("instance_id", env.InstanceId),
//					attribute.String("commit_sha", env.CwaCommitSha),
//				},
//			},
//		},
//		"WithOTLP/Enhanced": {
//			agentConfigPath: filepath.Join("agent_configs", "otlp-traces-config.json"),
//			generatorConfig: &base.TraceGeneratorConfig{
//				Interval: loadGeneratorInterval,
//				Annotations: map[string]interface{}{
//					"test_type":     "enhanced_otlp",
//					"instance_id":   env.InstanceId,
//					"commit_sha":    env.CwaCommitSha,
//					"service_name":  "otlp-trace-test",
//					"operation":     "test_operation",
//					"trace_version": "v2.0",
//				},
//				Metadata: map[string]map[string]interface{}{
//					"default": {
//						"custom_key":    "custom_value",
//						"test_metadata": "enhanced_test",
//						"environment":   "integration_test",
//					},
//					"http": {
//						"method": "POST",
//						"url":    "/api/test",
//						"status": 200,
//					},
//				},
//				Attributes: []attribute.KeyValue{
//					attribute.String("custom_key", "custom_value"),
//					attribute.String("test_type", "enhanced_otlp"),
//					attribute.String("instance_id", env.InstanceId),
//					attribute.String("commit_sha", env.CwaCommitSha),
//					attribute.String("service.name", "otlp-trace-test"),
//					attribute.String("service.version", "1.0.0"),
//					attribute.String("http.method", "POST"),
//					attribute.String("http.url", "/api/test"),
//					attribute.Int("http.status_code", 200),
//					attribute.Bool("test.enabled", true),
//				},
//			},
//		},
//	}
//	t.Logf("Sanity check: number of test cases:%d", len(testCases))
//	for name, testCase := range testCases {
//
//		t.Run(name, func(t *testing.T) {
//
//			OtlpTestCfg := base.TraceTestConfig{
//				Generator:       otlp.NewLoadGenerator(testCase.generatorConfig),
//				Name:            name,
//				AgentConfigPath: testCase.agentConfigPath,
//				AgentRuntime:    agentRuntime,
//			}
//			err := base.TraceTest(t, OtlpTestCfg)
//			require.NoError(t, err, "TraceTest failed because %s", err)
//
//		})
//	}
//}

type OtlpTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (t *OtlpTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Validate traces
	results = append(results, t.validateTraces())

	// Validate metrics
	for _, metricName := range t.GetMeasuredMetrics() {
		results = append(results, t.validateMetric(metricName))
	}

	// Validate logs
	results = append(results, t.validateLogs())

	return status.TestGroupResult{
		Name:        "UnifiedOTLP",
		TestResults: results,
	}
}

func (t *OtlpTestRunner) validateTraces() status.TestResult {
	annotations := map[string]interface{}{
		"test_type":   "unified_otlp",
		"instance_id": t.env.InstanceId,
	}

	traceIds, err := awsservice.GetTraceIDs(time.Now().Add(-5*time.Minute), time.Now(), awsservice.FilterExpression(annotations))
	if err != nil || len(traceIds) == 0 {
		return status.TestResult{Name: "OTLP_Traces", Status: status.FAILED, Reason: fmt.Errorf("no traces found: %v", err)}
	}
	return status.TestResult{Name: "OTLP_Traces", Status: status.SUCCESSFUL}
}

func (t *OtlpTestRunner) validateMetric(metricName string) status.TestResult {
	instructions := []dimension.Instruction{{
		Key:   "InstanceId",
		Value: dimension.ExpectedDimensionValue{Value: &t.env.InstanceId},
	}}

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.SUM, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *OtlpTestRunner) validateLogs() status.TestResult {
	testResult := status.TestResult{
		Name:   "OTLP_Logs",
		Status: status.FAILED,
	}

	// Wait for logs to be processed
	time.Sleep(30 * time.Second)

	since := time.Now().Add(-5 * time.Minute)
	until := time.Now()

	// Log stream name from config: {instance_id}-otlp
	logStreamName := fmt.Sprintf("%s-otlp", t.env.InstanceId)

	err := awsservice.ValidateLogs(
		logGroup,
		logStreamName,
		&since,
		&until,
		awsservice.AssertLogsNotEmpty(),
	)

	if err != nil {
		testResult.Reason = fmt.Errorf("log validation failed: %v", err)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *OtlpTestRunner) GetTestName() string                { return "UnifiedOTLP" }
func (t *OtlpTestRunner) GetAgentRunDuration() time.Duration { return testRuntime }
func (t *OtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_counter", "otlp_gauge"}
}
func (t *OtlpTestRunner) GetAgentConfigFileName() string { return "unified-otlp-config.json" }

func (t *OtlpTestRunner) SetupAfterAgentRun() error {
	// Start trace generation
	go t.generateTraces()

	// Start data generation via curl
	cmd := fmt.Sprintf(`while true; do
		curl -s -X POST http://127.0.0.1:4318/v1/metrics -H "Content-Type: application/json" -d '{
			"resourceMetrics": [{
				"scopeMetrics": [{
					"metrics": [{
						"name": "otlp_counter",
						"sum": {"dataPoints": [{"timeUnixNano": "'$(date +%%s%%N)'", "asInt": "42", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}], "isMonotonic": true}
					}, {
						"name": "otlp_gauge", 
						"gauge": {"dataPoints": [{"timeUnixNano": "'$(date +%%s%%N)'", "asDouble": 123.45, "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]}
					}]
				}]
			}]
		}';
		
		curl -s -X POST http://127.0.0.1:4318/v1/logs -H "Content-Type: application/json" -d '{
			"resourceLogs": [{
				"scopeLogs": [{
					"logRecords": [{
						"timeUnixNano": "'$(date +%%s%%N)'",
						"body": {"stringValue": "OTLP unified test log"},
						"attributes": [{"key": "instance_id", "value": {"stringValue": "%s"}}]
					}]
				}]
			}]
		}';
		
		sleep 15;
	done`, t.env.InstanceId, t.env.InstanceId, t.env.InstanceId)

	return common.RunAsyncCommand(cmd)
}

func (t *OtlpTestRunner) generateTraces() {
	generator := otlp.NewLoadGenerator(&base.TraceGeneratorConfig{
		Interval: 15 * time.Second,
		Annotations: map[string]interface{}{
			"test_type":   "unified_otlp",
			"instance_id": t.env.InstanceId,
		},
		Attributes: []attribute.KeyValue{
			attribute.String("test_type", "unified_otlp"),
			attribute.String("instance_id", t.env.InstanceId),
		},
	})
	generator.StartSendingTraces(context.Background())
}

var _ test_runner.ITestRunner = (*OtlpTestRunner)(nil)

func TestUnifiedOTLP(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &OtlpTestRunner{env: env}
	runner := &test_runner.TestRunner{TestRunner: testRunner}

	result := runner.Run()
	require.Equal(t, status.SUCCESSFUL, result.GetStatus(), "Unified OTLP test failed")
}
