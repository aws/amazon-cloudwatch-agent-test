// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type CollectDTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectDMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDTestRunner) GetTestName() string {
	return "CollectD"
}

func (t *CollectDTestRunner) GetAgentConfigFileName() string {
	return "collectd_config.json"
}

func (t *CollectDTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectDTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value", "collectd_counter_1_value"}
}

func (t *CollectDTestRunner) validateCollectDMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	instructions := []dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	}
	switch metricName {
	case "collectd_counter_1_value":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("counter")},
		})
	case "collectd_gauge_1_value":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("gauge")},
		})
	}

	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	if err != nil {
		return testResult
	}

	runDuration := t.GetAgentRunDuration()
	aggregationInterval := 30 * time.Second
	// If aggregation is not happening there could be a data point every 5 seconds.
	// So validate the upper bound.
	upperBound := int(runDuration/aggregationInterval) + 2
	// Allow 2 missing data points in case CW-Metrics-Web-Service has a 1 minute
	// delay to store.
	lowerBound := int(runDuration/aggregationInterval) - 2

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 1) {
		return testResult
	}

	instanceId := awsservice.GetInstanceId()

	requestBody := []byte(fmt.Sprintf(`{
	 "Namespace": "MetricValueBenchmarkTest",
     "MetricName": "statsd_timing_3",
     "Dimensions": [{
             "Name": "InstanceId",
             "Value": "%s"
         },
         {
             "Name": "key",
             "Value": "value"
         },
         {
             "Name": "metric_type",
             "Value": "timing"
         }]
}`, instanceId))

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, "us-west-2")
	if err != nil {
		return testResult
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return testResult
	}
	defer resp.Body.Close()

	// parse and verify the response
	var response struct {
		Entities []struct {
			KeyAttributes struct {
				Type        string `json:"Type"`
				Environment string `json:"Environment"`
				Name        string `json:"Name"`
			} `json:"KeyAttributes"`
		} `json:"Entities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return testResult
	}
	log.Printf("%v", response.Entities)

	output, err := common.RunCommand(fmt.Sprintf(`curl -i -X POST monitoring.us-west-2.amazonaws.com \
	-H 'Content-Type: application/json' \
	-H 'Content-Encoding: amz-1.0' \
	--user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
	-H "x-amz-security-token: $AWS_SESSION_TOKEN" \
	--aws-sigv4 "aws:amz:us-west-2:monitoring" \
	-H 'X-Amz-Target: com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric' \
	-d '{
		"Namespace": "MetricValueBenchmarkTest",
		"MetricName": "statsd_timing_3",
		"Dimensions": [{
				"Name": "InstanceId",
				"Value": "%s"
			},
			{
				"Name": "key",
				"Value": "value"
			},
			{
				"Name": "metric_type",
				"Value": "timing"
			}]
	}'`, instanceId))
	log.Printf(output)

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *CollectDTestRunner) GetAgentRunDuration() time.Duration {
	return time.Minute
}
