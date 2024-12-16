// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
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

func ValidateMetric(metricName, namespace string, dimensionsFilter []types.DimensionFilter) error {
	listMetricsInput := cloudwatch.ListMetricsInput{
		MetricName:     aws.String(metricName),
		Namespace:      aws.String(namespace),
		RecentlyActive: "PT3H",
		Dimensions:     dimensionsFilter,
	}
	data, err := CwmClient.ListMetrics(ctx, &listMetricsInput)
	if err != nil {
		return errors.New(fmt.Sprintf("Error getting metric data %v", err))
	}

	// Only validate if certain metrics are published by CloudWatchAgent in corresponding namespace
	// Since the metric value can be unpredictive.
	if len(data.Metrics) == 0 {
		dims := make([]metric, len(dimensionsFilter))
		for i, filter := range dimensionsFilter {
			dims[i] = metric{
				name:  *filter.Name,
				value: *filter.Value,
			}
		}
		return errors.New(fmt.Sprintf("No metrics found for dimension %v metric name %v namespace %v",
			dims, metricName, namespace))
	}

	return nil
}

// ValidateMetricWithTest takes the metric name, metric dimension and corresponding namespace that contains the metric
func ValidateMetricWithTest(t *testing.T, metricName, namespace string, dimensionsFilter []types.DimensionFilter, retries int, retryTime time.Duration) {
	var err error
	for i := 0; i < retries; i++ {
		err = ValidateMetric(metricName, namespace, dimensionsFilter)
		if err == nil {
			return
		}
		log.Printf("could not validate metrics try : %d of %d error %v", i+1, retries, err)
		time.Sleep(retryTime)
	}
	if err != nil {
		t.Errorf("could not validate metrics")
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
	data, err := CwmClient.GetMetricStatistics(ctx, &metricStatsInput)
	if err != nil {
		return false
	}

	dataPoints := 0
	log.Printf("These are the data points: %v", data)
	log.Printf("These are the data points: %v", data.Datapoints)

	for _, datapoint := range data.Datapoints {
		dataPoints = dataPoints + int(*datapoint.SampleCount)
	}
	log.Printf("Number of datapoints for start time %v with endtime %v and period %d is %d is inclusive between %d and %d", startTime, endTime, periodInSeconds, dataPoints, lowerBoundInclusive, upperBoundInclusive)

	if lowerBoundInclusive <= dataPoints && dataPoints <= upperBoundInclusive {
		return true
	}

	return false
}

func ValidateSampleCountFloat(metricName, namespace string, dimensions []types.Dimension,
	startTime time.Time, endTime time.Time,
	lowerBoundInclusive float64, upperBoundInclusive float64, periodInSeconds int32) bool {

	metricStatsInput := cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String(metricName),
		Namespace:  aws.String(namespace),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(periodInSeconds),
		Dimensions: dimensions,
		Statistics: []types.Statistic{types.StatisticSampleCount},
	}
	data, err := CwmClient.GetMetricStatistics(ctx, &metricStatsInput)
	if err != nil {
		return false
	}

	var dataPoints float64
	log.Printf("These are the data points: %v", data)
	log.Printf("These are the data points: %v", data.Datapoints)

	for _, datapoint := range data.Datapoints {
		dataPoints += *datapoint.SampleCount
	}
	log.Printf("Number of datapoints for start time %v with endtime %v and period %d is %.3f is inclusive between %.3f and %.3f",
		startTime, endTime, periodInSeconds, dataPoints, lowerBoundInclusive, upperBoundInclusive)

	if lowerBoundInclusive <= dataPoints && dataPoints <= upperBoundInclusive {
		return true
	}

	return false
}

func GetMetricStatistics(
	metricName string,
	namespace string,
	dimensions []types.Dimension,
	startTime time.Time,
	endTime time.Time,
	periodInSeconds int32,
	statType []types.Statistic,
	extendedStatType []string,
) (*cloudwatch.GetMetricStatisticsOutput, error) {
	metricStatsInput := cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String(metricName),
		Namespace:  aws.String(namespace),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(periodInSeconds),
		Dimensions: dimensions,
	}
	// GetMetricStatistics can only have either Statistics or ExtendedStatistics, not both
	if extendedStatType == nil {
		metricStatsInput.Statistics = statType
	} else {
		metricStatsInput.ExtendedStatistics = extendedStatType
	}

	return CwmClient.GetMetricStatistics(ctx, &metricStatsInput)
}

// GetMetricData takes the metric name, metric dimension and metric namespace and return the query metrics
func GetMetricData(metricDataQueries []types.MetricDataQuery, startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {
	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	data, err := CwmClient.GetMetricData(ctx, &getMetricDataInput)
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

// ReportMetric sends a single metric to CloudWatch.
// Does not support sending dimensions.
func ReportMetric(namespace string,
	name string,
	value float64,
	units types.StandardUnit,
) error {
	_, err := CwmClient.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(name),
				Value:      aws.Float64(value),
				Unit:       units,
			},
		},
	})
	return err
}
