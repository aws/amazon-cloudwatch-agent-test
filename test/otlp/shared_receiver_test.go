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
	testRuntime = 4 * time.Minute
	namespace   = "CWAgent/OTLP"
	logGroup    = "/aws/amazon-cloudwatch-agent/otlp"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type UnifiedOTLPTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (t *UnifiedOTLPTestRunner) Validate() status.TestGroupResult {
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

func (t *UnifiedOTLPTestRunner) validateTraces() status.TestResult {
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

func (t *UnifiedOTLPTestRunner) validateMetric(metricName string) status.TestResult {
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

func (t *UnifiedOTLPTestRunner) validateLogs() status.TestResult {
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

func (t *UnifiedOTLPTestRunner) GetTestName() string                { return "UnifiedOTLP" }
func (t *UnifiedOTLPTestRunner) GetAgentRunDuration() time.Duration { return testRuntime }
func (t *UnifiedOTLPTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_counter", "otlp_gauge"}
}
func (t *UnifiedOTLPTestRunner) GetAgentConfigFileName() string { return "unified-otlp-config.json" }

func (t *UnifiedOTLPTestRunner) SetupAfterAgentRun() error {
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

func (t *UnifiedOTLPTestRunner) generateTraces() {
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

var _ test_runner.ITestRunner = (*UnifiedOTLPTestRunner)(nil)

func TestUnifiedOTLP(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &UnifiedOTLPTestRunner{env: env}
	runner := &test_runner.TestRunner{TestRunner: testRunner}

	result := runner.Run()
	require.Equal(t, status.SUCCESSFUL, result.GetStatus(), "Unified OTLP test failed")
}
