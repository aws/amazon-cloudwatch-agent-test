// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type cloudwatchAPI interface {
	// isMetricExist returns Instance Private IP Address
	ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error

	ValidateSampleCount(metricName, namespace string, dimensions []types.Dimension, startTime, endTime time.Time, lowerBoundInclusive, upperBoundInclusive int, periodInSeconds int32) error
}

type cloudwatchConfig struct {
	cxt      context.Context
	cwClient *cloudwatch.Client
}

func NewCloudWatchConfig(cfg aws.Config, cxt context.Context) cloudwatchAPI {
	cwClient := cloudwatch.NewFromConfig(cfg)
	return &cloudwatchConfig{
		cxt:      cxt,
		cwClient: cwClient,
	}
}

// ValidateMetrics takes the metric name, metric dimension and corresponding namespace that contains the metric
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
		return fmt.Errorf("Number of datapoints for start time %v with endtime %v and period %d is %d which is expected to be between %d and %d",
			startTime, endTime, periodInSeconds, dataPoints, lowerBoundInclusive, upperBoundInclusive)
	}

	return nil
}
