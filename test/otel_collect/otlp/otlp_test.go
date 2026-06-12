// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	otlpRuntime  = 3 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"
)

type OtlpCollectTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*OtlpCollectTestRunner)(nil)

func (t *OtlpCollectTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics())
}

func (t *OtlpCollectTestRunner) GetTestName() string                { return "OtelCollectOTLP" }
func (t *OtlpCollectTestRunner) GetAgentRunDuration() time.Duration { return otlpRuntime }
func (t *OtlpCollectTestRunner) GetAgentConfigFileName() string     { return "otlp_config.json" }
func (t *OtlpCollectTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_test_counter", "otlp_test_gauge"}
}

func (t *OtlpCollectTestRunner) SetupAfterAgentRun() error {
	go t.sendTestMetrics()
	return nil
}

func (t *OtlpCollectTestRunner) sendTestMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(otlpRuntime - 30*time.Second)
	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			payload := buildOtlpMetricsPayload(t.env.InstanceId)
			req, _ := http.NewRequest("POST", otlpEndpoint+"/v1/metrics", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			http.DefaultClient.Do(req) //nolint:errcheck
		}
	}
}

func buildOtlpMetricsPayload(instanceId string) []byte {
	now := time.Now().UnixNano()
	payload := fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]},
    "scopeMetrics": [{
      "metrics": [
        {
          "name": "otlp_test_counter",
          "sum": {
            "dataPoints": [{"asInt": "1", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}],
            "isMonotonic": true,
            "aggregationTemporality": 2
          }
        },
        {
          "name": "otlp_test_gauge",
          "gauge": {
            "dataPoints": [{"asDouble": 42.0, "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        }
      ]
    }]
  }]
}`, instanceId, now, instanceId, now, instanceId)
	return []byte(payload)
}

func TestOTLPCollect(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &OtlpCollectTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}
