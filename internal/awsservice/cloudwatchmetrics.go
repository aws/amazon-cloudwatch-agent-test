// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// ValidateMetric takes the metric name, metric dimension and metric namespace to know whether a metric is exist based on the previous parametersfunc (c *cloudwatchConfig) ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
func ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {

	data, err := CwmClient.ListMetrics(ctx, &cloudwatch.ListMetricsInput{
		MetricName:     aws.String(metricName),
		Namespace:      aws.String(namespace),
		RecentlyActive: "PT3H",
		Dimensions:     dimensionsFilter,
	})
	if err != nil {
		return err
	}

	if len(data.Metrics) == 0 {
		return fmt.Errorf("no metric %s is found with namespace %s", metricName, namespace)
	}

	return nil
}

// IsMetricSampleCountWithinBound checking if certain metric's sample count is within the predefined bound interval
func IsMetricSampleCountWithinBound(
	metricName, namespace string,
	dimensions []types.Dimension,
	startTime, endTime time.Time,
	lowerBoundInclusive, upperBoundInclusive int,
	periodInSeconds int32,
) bool {
	data, err := CwmClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String(metricName),
		Namespace:  aws.String(namespace),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(periodInSeconds),
		Dimensions: dimensions,
		Statistics: []types.Statistic{types.StatisticSampleCount},
	})
	if err != nil {
		return false
	}

	dataPoints := 0

	for _, datapoint := range data.Datapoints {
		dataPoints = dataPoints + int(*datapoint.SampleCount)
	}

	log.Printf("Number of datapoints for start time %v with endtime %v and period %d is %d is inclusive between %d and %d", startTime, endTime, periodInSeconds, dataPoints, lowerBoundInclusive, upperBoundInclusive)

	if !(lowerBoundInclusive <= dataPoints) || !(upperBoundInclusive >= dataPoints) {
		return false
	}

	return true
}

// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
func GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {

	data, err := CwmClient.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	})
	if err != nil {
		return nil, err
	}

	return data, nil
}
