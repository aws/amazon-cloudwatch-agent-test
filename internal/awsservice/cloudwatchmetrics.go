// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type cwmAPI interface {
	// ValidateMetric takes the metric name, metric dimension and metric namespace to know whether a metric is exist based on the previous parameters
	ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error

	// IsMetricSampleCountWithinBoundInclusive checking if certain metric's sample count is within the predefined bound interval
	IsMetricSampleCountWithinBoundInclusive(metricName, namespace string, dimensions []types.Dimension, startTime, endTime time.Time, lowerBoundInclusive, upperBoundInclusive int, periodInSeconds int32) bool

	// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
	GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error)
}

type cloudwatchSDK struct {
	cxt      context.Context
	cwClient *cloudwatch.Client
}

func NewCloudWatchSDKClient(cfg aws.Config, cxt context.Context) cwmAPI {
	cwClient := cloudwatch.NewFromConfig(cfg)
	return &cloudwatchSDK{
		cxt:      cxt,
		cwClient: cwClient,
	}
}

// ValidateMetric takes the metric name, metric dimension and metric namespace to know whether a metric is exist based on the previous parametersfunc (c *cloudwatchConfig) ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
func (c *cloudwatchSDK) ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
	listMetricsInput := cloudwatch.ListMetricsInput{
		MetricName:     aws.String(metricName),
		Namespace:      aws.String(namespace),
		RecentlyActive: "PT3H",
		Dimensions:     dimensionsFilter,
	}

	data, err := c.cwClient.ListMetrics(c.cxt, &listMetricsInput)
	if err != nil {
		return err
	}

	if len(data.Metrics) == 0 {
		return fmt.Errorf("no metric %s is found with namespace %s", metricName, namespace)
	}

	return nil
}

// isMetricSampleCountWithinBoundInclusive checking if certain metric's sample count is within the predefined bound interval
func (c *cloudwatchSDK) IsMetricSampleCountWithinBoundInclusive(
	metricName, namespace string,
	dimensions []types.Dimension,
	startTime, endTime time.Time,
	lowerBoundInclusive, upperBoundInclusive int,
	periodInSeconds int32,
) bool {
	metricStatsInput := cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String(metricName),
		Namespace:  aws.String(namespace),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(periodInSeconds),
		Dimensions: dimensions,
		Statistics: []types.Statistic{types.StatisticSampleCount},
	}
	data, err := c.cwClient.GetMetricStatistics(c.cxt, &metricStatsInput)

	if err != nil {
		return false
	}

	dataPoints := 0

	for _, datapoint := range data.Datapoints {
		dataPoints = dataPoints + int(*datapoint.SampleCount)
	}

	if !(lowerBoundInclusive <= dataPoints) || !(upperBoundInclusive >= dataPoints) {
		return false
	}

	return true
}

// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
func (c *cloudwatchSDK) GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {
	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	data, err := c.cwClient.GetMetricData(c.cxt, &getMetricDataInput)
	if err != nil {
		return nil, err
	}

	return data, nil
}
