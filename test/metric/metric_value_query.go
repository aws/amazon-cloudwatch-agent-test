// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type MetricValueFetcher struct {
	Env                    *environment.MetaData
	ExpectedDimensionNames []string
}

func (n *MetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := dimension.GetMetricDefaultDimensions(*n.Env, metricName)
	expectedDimensions := dimension.GetExpectedDimensions(n.Env, n.ExpectedDimensionNames)
	dimensions = append(dimensions, expectedDimensions...)
	values, err := n.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %v: %v", metricName, err.Error())
	}
	return values, err
}

func (n *MetricValueFetcher) fetch(namespace, metricName string, metricSpecificDimensions []types.Dimension, stat Statistics) (MetricValues, error) {
	dimensions := metricSpecificDimensions
	log.Printf("Metric query input dimensions : %s", fmt.Sprint(dimensions))

	metricToFetch := types.Metric{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: dimensions,
	}

	metricQueryPeriod := int32(60)
	metricDataQueries := []types.MetricDataQuery{
		{
			MetricStat: &types.MetricStat{
				Metric: &metricToFetch,
				Period: &metricQueryPeriod,
				Stat:   aws.String(string(stat)),
			},
			Id: aws.String(metricName),
		},
	}

	endTime := time.Now()
	startTime := subtractMinutes(endTime, 10)
	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	log.Printf("Metric data input is : %s", fmt.Sprint(getMetricDataInput))

	cwmClient, clientContext, err := test.GetCloudWatchMetricsClient()
	if err != nil {
		return nil, fmt.Errorf("Error occurred while creating CloudWatch client: %v", err.Error())
	}

	output, err := cwmClient.GetMetricData(*clientContext, &getMetricDataInput)
	if err != nil {
		return nil, fmt.Errorf("Error getting metric data %v", err)
	}

	result := output.MetricDataResults[0].Values
	log.Printf("Metric values are : %s", fmt.Sprint(result))

	return result, nil
}

func subtractMinutes(fromTime time.Time, minutes int) time.Time {
	tenMinutes := time.Duration(-1*minutes) * time.Minute
	return fromTime.Add(tenMinutes)
}
