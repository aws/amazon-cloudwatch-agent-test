// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

var (
	metricsCtx context.Context
	cwm        *cloudwatch.Client
)

const (
	instanceId   = "InstanceId"
	appendMetric = "append"
	loremIpsum   = "Lorem ipsum dolor sit amet consectetur adipiscing elit Vivamus non mauris malesuada mattis ex eget porttitor purus Suspendisse potenti Praesent vel sollicitudin ipsum Quisque luctus pretium lorem non faucibus Ut vel quam dui Nunc fermentum condimentum consectetur Morbi tellus mauris tristique tincidunt elit consectetur hendrerit placerat dui In nulla erat finibus eget erat a hendrerit sodales urna In sapien purus auctor sit amet congue ut congue eget nisi Vivamus sed neque ut ligula lobortis accumsan quis id metus In feugiat velit et leo mattis non fringilla dui elementum Proin a nisi ac sapien vulputate consequat Vestibulum eu tellus mi Integer consectetur efficitur"
)

// TODO: Refactor Structure and Interface for more easier follow that shares the same session
type metric struct {
	name  string
	value string
}

// ValidateMetrics takes the metric name, metric dimension and corresponding namespace that contains the metric
func ValidateMetrics(t *testing.T, metricName, namespace string, dimensionsFilter []types.DimensionFilter) {
	listMetricsInput := cloudwatch.ListMetricsInput{
		MetricName:     aws.String(metricName),
		Namespace:      aws.String(namespace),
		RecentlyActive: "PT3H",
		Dimensions:     dimensionsFilter,
	}
	data, err := CwmClient.ListMetrics(cxt, &listMetricsInput)
	if err != nil {
		t.Errorf("Error getting metric data %v", err)
	}

	// Only validate if certain metrics are published by CloudWatchAgent in corresponding namespace
	// Since the metric value can be unpredictive.
	if len(data.Metrics) == 0 {
		metrics := make([]metric, len(dimensionsFilter))
		for i, filter := range dimensionsFilter {
			metrics[i] = metric{
				name:  *filter.Name,
				value: *filter.Value,
			}
		}
		t.Errorf("No metrics found for dimension %v metric name %v namespace %v",
			metrics, metricName, namespace)
	}

}

func ValidateSampleCount(metricName, namespace string, dimensions []types.Dimension,
	startTime time.Time, endTime time.Time,
	lowerBoundInclusive int, upperBoundInclusive int, periodInSeconds int32) bool {

	metricStatsInput := cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String(metricName),
		Namespace:  aws.String(namespace),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(periodInSeconds),
		Dimensions: dimensions,
		Statistics: []types.Statistic{types.StatisticSampleCount},
	}
	data, err := CwmClient.GetMetricStatistics(cxt, &metricStatsInput)
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

	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	data, err := CwmClient.GetMetricData(cxt, &getMetricDataInput)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func BuildDimensionFilterList(appendDimension int) []types.DimensionFilter {
	// we append dimension from 0 to max number - 2
	// then we add dimension instance id
	// thus for max dimension 10, 0 to 8 + instance id = 10 dimension
	ec2InstanceId := GetInstanceId()
	dimensionFilter := make([]types.DimensionFilter, appendDimension)
	for i := 0; i < appendDimension-1; i++ {
		dimensionFilter[i] = types.DimensionFilter{
			Name:  aws.String(fmt.Sprintf("%s%d", appendMetric, i)),
			Value: aws.String(fmt.Sprintf("%s%d", loremIpsum+appendMetric, i)),
		}
	}
	dimensionFilter[appendDimension-1] = types.DimensionFilter{
		Name:  aws.String(instanceId),
		Value: aws.String(ec2InstanceId),
	}
	return dimensionFilter
}
