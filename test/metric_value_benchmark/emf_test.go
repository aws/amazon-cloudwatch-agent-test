// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/qri-io/jsonschema"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type EMFTestRunner struct {
	test_runner.BaseTestRunner
}

//go:embed agent_resources/emf_counter.json
var emfMetricValueBenchmarkSchema string

var _ test_runner.ITestRunner = (*EMFTestRunner)(nil)

func (t *EMFTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEMFMetric(metricName)
	}

	testResults = append(testResults, validateEMFLogs("MetricValueBenchmarkTest", awsservice.GetInstanceId()))

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EMFTestRunner) GetTestName() string {
	return "EMF"
}

func (t *EMFTestRunner) GetAgentConfigFileName() string {
	return "emf_config.json"
}

func (t *EMFTestRunner) SetupAfterAgentRun() error {
	// EC2 Image Builder creates a bash script that sends emf format to cwagent at port 8125
	// The bash script is at /etc/emf.sh
	// TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
	// INSTANCEID=$(curl -H "X-aws-ec2-metadata-token: \${TOKEN}" -v http://169.254.169.254/latest/meta-data/instance-id)
	// for times in  {1..3}
	//  do
	//   CURRENT_TIME=$(date +%s%N | cut -b1-13)
	//   echo '{"_aws":{"Timestamp":'"${CURRENT_TIME}"',"LogGroupName":"MetricValueBenchmarkTest","CloudWatchMetrics":[{"Namespace":"MetricValueBenchmarkTest","Dimensions":[["Type","InstanceId"]],"Metrics":[{"Name":"EMFCounter","Unit":"Count","InstanceId":"'"${INSTANCEID}"'"}]}]},"Type":"Counter","EMFCounter":5}' \ > /dev/udp/0.0.0.0/25888
	//   sleep 5
	// done
	startEMFCommands := []string{
		"sudo bash /etc/emf.sh",
	}

	return common.RunCommands(startEMFCommands)
}

func (t *EMFTestRunner) GetMeasuredMetrics() []string {
	return []string{"EMFCounter"}
}

func (t *EMFTestRunner) validateEMFMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "Type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("Counter")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 5) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func validateEMFLogs(group, stream string) status.TestResult {
	testResult := status.TestResult{
		Name:   "emf-logs",
		Status: status.FAILED,
	}

	rs := jsonschema.Must(emfMetricValueBenchmarkSchema)

	validateLogContents := func(s string) bool {
		return strings.Contains(s, "\"EMFCounter\":5")
	}

	now := time.Now()
	ok, err := awsservice.ValidateLogs(group, stream, nil, &now, func(logs []string) bool {
		if len(logs) < 1 {
			return false
		}

		for _, l := range logs {
			if !awsservice.MatchEMFLogWithSchema(l, rs, validateLogContents) {
				return false
			}
		}
		return true
	})

	if err != nil || !ok {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
