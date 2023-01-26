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

	// ValidateSampleCount checking if certain metric's sample count is within the predefined bound interval
	ValidateSampleCount(metricName, namespace string, dimensions []types.Dimension, startTime, endTime time.Time, lowerBoundInclusive, upperBoundInclusive int, periodInSeconds int32) error

	// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
	GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error)
}

type cloudwatchConfig struct {
	cxt      context.Context
	cwClient *cloudwatch.Client
}

func NewCloudWatchConfig(cfg aws.Config, cxt context.Context) cwmAPI {
	cwClient := cloudwatch.NewFromConfig(cfg)
	return &cloudwatchConfig{
		cxt:      cxt,
		cwClient: cwClient,
	}
}

// ValidateMetric takes the metric name, metric dimension and metric namespace to know whether a metric is exist based on the previous parametersfunc (c *cloudwatchConfig) ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
func (c *cloudwatchConfig) ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
	listMetricsInput := cloudwatch.ListMetricsInput{
		MetricName:     aws.String(metricName),
		Namespace:      aws.String(namespace),
		RecentlyActive: "PT3H",
		Dimensions:     dimensionsFilter,
	}

	_, err := c.cwClient.ListMetrics(c.cxt, &listMetricsInput)
	if err != nil {
		return err
	}

	return nil
}

func (c *cloudwatchConfig) ValidateSampleCount(
	metricName, namespace string,
	dimensions []types.Dimension,
	startTime, endTime time.Time,
	lowerBoundInclusive, upperBoundInclusive int,
	periodInSeconds int32,
) error {

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
		return err
	}

	dataPoints := 0

	for _, datapoint := range data.Datapoints {
		dataPoints = dataPoints + int(*datapoint.SampleCount)
	}

	if !(lowerBoundInclusive <= dataPoints) || !(upperBoundInclusive >= dataPoints) {
		return fmt.Errorf("number of datapoints for start time %v with endtime %v and period %d is %d which is expected to be between %d and %d",
			startTime, endTime, periodInSeconds, dataPoints, lowerBoundInclusive, upperBoundInclusive)
	}

	return nil
}

// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
func (c *cloudwatchConfig) GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {
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
