// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux
// +build linux

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type EMFTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*EMFTestRunner)(nil)

func (t *EMFTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEMFMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *EMFTestRunner) getTestName() string {
	return "EMF"
}

func (t *EMFTestRunner) getAgentConfigFileName() string {
	return "emf_config.json"
}

func (t *EMFTestRunner) getAgentRunDuration() time.Duration {
	return time.Minute
}

func (t *EMFTestRunner) setupAfterAgentRun() error {
	// EC2 Image Builder creates a bash script that sends emf format to cwagent at port 8125
	// The bash script is at /etc/emf.sh
	// TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
	// INSTANCEID=$(curl -H "X-aws-ec2-metadata-token: \${TOKEN}" -v http://169.254.169.254/latest/meta-data/instance-id)
	// for times in  {1..3}
	//  do
	//   CURRENT_TIME=$(date +%s%N | cut -b1-13)
	//   echo '{"_aws":{"Timestamp":'"${CURRENT_TIME}"',"LogGroupName":"MetricValueBenchmarkTest","CloudWatchMetrics":[{"Namespace":"MetricValueBenchmarkTest","Dimensions":[["Type","InstanceId"]],"Metrics":[{"Name":"EMFCounter","Unit":"Count","InstanceId":"'"${INSTANCEID}"'"}]}]},"Type":"Counter","EMFCounter":5}' \ > /dev/udp/0.0.0.0/25888
	//   sleep 60
	// done
	startEMFCommands := []string{
		"sudo bash /etc/emf.sh",
	}

	return common.RunCommands(startEMFCommands)
}

func (t *EMFTestRunner) getMeasuredMetrics() []string {
	return []string{"EMFCounter"}
}

func (t *EMFTestRunner) validateEMFMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := t.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}
