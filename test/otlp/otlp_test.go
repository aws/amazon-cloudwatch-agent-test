// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

const (
	namespace             = "CWAgent/OTLP"
	logGroup              = "/aws/cwagent"
	agentRuntime          = 3 * time.Minute
	loadGeneratorInterval = 5 * time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type OtlpTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (t *OtlpTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Validate traces
	var result status.TestResult
	result = t.validateTraces()
	fmt.Printf("traces result: %s\n", result.Status)
	results = append(results, result)

	// Validate metrics
	for _, metricName := range t.GetMeasuredMetrics() {
		result = t.validateMetric(metricName)
		fmt.Printf("metric (%s) result: %s\n", metricName, result.Status)
		results = append(results, result)
	}

	// Validate logs
	result = t.validateLogs()
	fmt.Printf("logs result: %s\n", result.Status)
	results = append(results, result)

	return status.TestGroupResult{
		Name:        "OTLP",
		TestResults: results,
	}
}

func (t *OtlpTestRunner) validateTraces() status.TestResult {
	annotations := map[string]interface{}{
		"test_type":   "otlp",
		"instance_id": t.env.InstanceId,
	}

	err := base.ValidateTraceSegments(
		time.Now().Add(-5*time.Minute),
		time.Now(),
		annotations,
		nil,
	)

	if err != nil {
		return status.TestResult{Name: "OTLP_Traces", Status: status.FAILED, Reason: err}
	}

	return status.TestResult{Name: "OTLP_Traces", Status: status.SUCCESSFUL}
}

func (t *OtlpTestRunner) validateMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})
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

	streams := awsservice.GetLogStreams(logGroup)
	if len(streams) == 0 {
		testResult.Reason = fmt.Errorf("no log streams found")
		return testResult
	}

	since := time.Now().Add(-agentRuntime)
	until := time.Now()
	err := awsservice.ValidateLogs(
		logGroup,
		*streams[0].LogStreamName,
		&since,
		&until,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"InstanceId\":\"%s\"", t.env.InstanceId)),
			func(event types.OutputLogEvent) error {
				if strings.Contains(*event.Message, "\"test.type\":\"otlp_integration_logs_test\"") && !strings.Contains(*event.Message, "\"otlp_emf_") {
					return fmt.Errorf("log event message missing substring (%s): %s", "otlp_emf", *event.Message)
				}
				return nil
			},
		),
	)
	if err != nil {
		testResult.Reason = err
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *OtlpTestRunner) GetTestName() string                { return "OTLP" }
func (t *OtlpTestRunner) GetAgentRunDuration() time.Duration { return agentRuntime }
func (t *OtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_counter", "otlp_gauge"}
}
func (t *OtlpTestRunner) GetAgentConfigFileName() string { return "shared_config.json" }

func (t *OtlpTestRunner) SetupAfterAgentRun() error {
	// trace generation
	go t.generateTraces()

	cmd := exec.Command("/bin/bash", "resources/otlp_pusher.sh")
	cmd.Env = append(os.Environ(), fmt.Sprintf("INSTANCE_ID=%s", t.env.InstanceId))
	return cmd.Start()
}

func (t *OtlpTestRunner) generateTraces() {
	generator := otlp.NewLoadGenerator(&base.TraceGeneratorConfig{
		Interval: 15 * time.Second,
		Annotations: map[string]interface{}{
			"test_type":   "otlp",
			"instance_id": t.env.InstanceId,
			"commit_sha":  t.env.CwaCommitSha,
		},
		Attributes: []attribute.KeyValue{
			attribute.String("test_type", "otlp"),
			attribute.String("instance_id", t.env.InstanceId),
		},
	})
	generator.StartSendingTraces(context.Background())
}

var _ test_runner.ITestRunner = (*OtlpTestRunner)(nil)

func TestOTLP(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	// shared receiver test
	testRunner := &OtlpTestRunner{env: env}
	runner := &test_runner.TestRunner{TestRunner: testRunner}

	result := runner.Run()
	require.Equal(t, status.SUCCESSFUL, result.GetStatus(), result.TestResults[0].Reason)

	// traces only tests
	testCases := map[string]struct {
		agentConfigPath string
		generatorConfig *base.TraceGeneratorConfig
	}{
		"WithOTLP/Simple": {
			agentConfigPath: filepath.Join("agent_configs", "default_traces.json"),
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
