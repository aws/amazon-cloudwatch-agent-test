// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package amp

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

// NOTE: this should match with append_dimensions under metrics in agent config
var appendDims = map[string]string{
	"d1": "foo",
	"d2": "bar",
}

var metadata *environment.MetaData

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &HistogramTestRunner{},
		},
	}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestHistogramTestSuite(t *testing.T) {
	suite.Run(t, new(HistogramTestSuite))
}

type HistogramTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *HistogramTestSuite) GetSuiteName() string {
	return "Histogram to CW"
}

func (suite *HistogramTestSuite) TestAllInSuite() {
	metadata = environment.GetEnvironmentMetaData()

	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Assume Role Test Suite Failed")
}

type HistogramTestRunner struct {
	test_runner.BaseTestRunner
}

func (t HistogramTestRunner) GetTestName() string {
	return "otlp_histograms"
}

func (t HistogramTestRunner) GetAgentConfigFileName() string {
	return "otlp_config.json"
}

func (t HistogramTestRunner) Validate() status.TestGroupResult {
	return t.validateOtlpHistogramMetrics()
}

func (t *HistogramTestRunner) validateOtlpHistogramMetrics() status.TestGroupResult {
	histogramMetrics := t.getOtlpHistogramMetrics()
	testResults := make([]status.TestResult, len(histogramMetrics))

	for i, metricName := range histogramMetrics {
		testResults[i] = t.validateHistogramMetric(metricName, []types.Dimension{})
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *HistogramTestRunner) validateHistogramMetric(metricName string, dims []types.Dimension) status.TestResult {
	namespace := ""

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.Statistics("todo"), metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t HistogramTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}

func (t HistogramTestRunner) getOtlpHistogramMetrics() []string {
	return []string{
		"my_cumulative_histogram",
		"my_delta_histogram",
	}
}

func (t HistogramTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	// replace AMP workspace ID placeholder with a testing workspace ID from metadata
	agentConfigPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	ampCommands := []string{
		"sed -ie 's/{workspace_id}/" + metadata.AmpWorkspaceId + "/g' " + agentConfigPath,
		// use below to add JMX metrics then update agent config & GetMeasuredMetrics()
		//"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030  -Dcom.sun.management.jmxremote.host=0.0.0.0  -Djava.rmi.server.hostname=0.0.0.0 -Dserver.port=8090 -Dspring.application.admin.enabled=true -jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}
	err = common.RunCommands(ampCommands)
	if err != nil {
		return fmt.Errorf("failed to modify agent configuration: %w", err)
	}

	return t.SetUpConfig()
}

func (t HistogramTestRunner) SetupAfterAgentRun() error {
	// OTLP source has some special setup after the agent starts
	return common.RunAsyncCommand("resources/otlp_pusher.sh")
}

func getDimensions() []types.Dimension {
	factory := dimension.GetDimensionFactory(*metadata)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceType",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}

func matchDimensions(labels map[string]interface{}) bool {
	if len(appendDims) > len(labels) {
		return false
	}
	for k, v := range appendDims {
		if lv, found := labels[k]; !found || lv != v {
			return false
		}
	}
	return true
}

var _ test_runner.ITestRunner = (*HistogramTestRunner)(nil)
