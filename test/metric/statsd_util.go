// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// Must match JSON config
	statsdMetricsAggregationInterval = 30 * time.Second
	statsdMetricsCollectionInterval  = 5 * time.Second
)

var (
	// metric name must use "_"" as a separator and include the metric type.
	StatsdMetricNames = []string{
		"statsd_counter_1",
		"statsd_gauge_2",
		"statsd_timing_3",
		"statsd_counter_4",
		"statsd_gauge_5",
		"statsd_timing_6",
	}
	StatsdMetricValues = []float64{1000, 2000, 3000, 4000, 5000, 6000}
)

func ValidateStatsdMetric(dimFactory dimension.Factory, namespace string, dimensionKey string, metricName string, expectedValue float64, runDuration time.Duration, sendInterval time.Duration) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	instructions := []dimension.Instruction{
		{
			Key:   dimensionKey,
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "key",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("value")},
		},
	}
	split := strings.Split(metricName, "_")
	if len(split) != 3 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
	instructions = append(instructions, dimension.Instruction{
		// CWA adds this metric_type dimension.
		Key:   "metric_type",
		Value: dimension.ExpectedDimensionValue{Value: aws.String(metricType)},
	})
	dims, failed := dimFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := MetricValueFetcher{}
	// Check average.
	values, err := fetcher.Fetch(namespace, metricName, dims, AVERAGE, HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	lowerBound := int(runDuration/statsdMetricsAggregationInterval) - 4
	if len(values) < lowerBound {
		log.Printf("fail: lowerBound %v, actual %v", lowerBound, len(values))
		return testResult
	}
	// Counters get summed up over the metrics_collection_interval.
	if metricType == "counter" {
		expectedValue *= float64(statsdMetricsCollectionInterval / sendInterval)
	}
	if !IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedValue)) {
		return testResult
	}
	// Check aggregation by checking sample count.
	// Expect samples to be metrics_aggregation_interval / metrics_collection_interval
	expectedSampleCount := statsdMetricsAggregationInterval / statsdMetricsCollectionInterval
	if metricType == "timing" {
		// Every single timing is counted.
		// Sent twice per send_interval.
		expectedSampleCount = 2 * statsdMetricsAggregationInterval / sendInterval
	}
	values, err = fetcher.Fetch(namespace, metricName, dims, SAMPLE_COUNT, HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}
	// Skip check on the last value.
	values = values[:len(values)-1]
	if !IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedSampleCount)) {
		return testResult
	}

	err = ValidateStatsdEntity()
	if err != nil {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func ValidateStatsdEntity() error {
	// build request
	instanceId := awsservice.GetInstanceId()
	requestBody := []byte(fmt.Sprintf(`{
	 "Namespace": "MetricValueBenchmarkTest",
     "MetricName": "statsd_timing_3",
     "Dimensions":
	 	[{
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
		return err
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read and print response status
	fmt.Printf("Response Status: %s\n", resp.Status)

	// Read and print response headers
	fmt.Println("Response Headers:")
	for key, values := range resp.Header {
		fmt.Printf("%s: %v\n", key, values)
	}

	// Read and print response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}
	fmt.Printf("Response Body: %s\n", string(body))

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
		return err
	}

	_, err = common.RunCommand(fmt.Sprintf(`curl -i -X POST monitoring.us-west-2.amazonaws.com \
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

	if err != nil {
		return err
	}

	return nil
}
