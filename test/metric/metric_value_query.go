// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type MetricValueFetcher struct {
}

type ExtendedStatistics string

const (
	P50 ExtendedStatistics = "p50"
	P90 ExtendedStatistics = "p90"
	P95 ExtendedStatistics = "p95"
	P99 ExtendedStatistics = "p99"
)

func logDimensions(dims []types.Dimension) {
	log.Printf("\tDimensions:\n")
	for _, d := range dims {
		if d.Name != nil && d.Value != nil {
			log.Printf("\t\tDim(name=%q, val=%q)\n", *d.Name, *d.Value)
		}
	}
}

func (n *MetricValueFetcher) Fetch(namespace, metricName string, metricSpecificDimensions []types.Dimension, stat Statistics, metricQueryPeriod int32) (MetricValues, error) {
	dimensions := metricSpecificDimensions
	log.Println("Metric query input dimensions")
	logDimensions(dimensions)
	metricToFetch := types.Metric{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: dimensions,
	}

	metricDataQueries := []types.MetricDataQuery{
		{
			MetricStat: &types.MetricStat{
				Metric: &metricToFetch,
				Period: &metricQueryPeriod,
				Stat:   aws.String(string(stat)),
			},
			Id: aws.String(strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(metricName, "-", "_"), ".", "_"))),
		},
	}

	endTime := time.Now()
	startTime := subtractMinutes(endTime, 10)
	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	log.Printf("Metric data input: namespace %v, name %v, stat %v, period %v",
		namespace, metricName, stat, metricQueryPeriod)

	output, err := awsservice.CwmClient.GetMetricData(context.Background(), &getMetricDataInput)
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

func (n *MetricValueFetcher) FetchExtended(namespace, metricName string, metricSpecificDimensions []types.Dimension, extendedStats []string, metricQueryPeriod int32) (map[string]MetricValues, error) {
	dimensions := metricSpecificDimensions
	log.Println("Metric query input dimensions for extended statistics")
	logDimensions(dimensions)

	// Create metric data queries for each extended statistic
	metricDataQueries := make([]types.MetricDataQuery, len(extendedStats))
	for i, stat := range extendedStats {
		metricToFetch := types.Metric{
			Namespace:  aws.String(namespace),
			MetricName: aws.String(metricName),
			Dimensions: dimensions,
		}

		metricDataQueries[i] = types.MetricDataQuery{
			MetricStat: &types.MetricStat{
				Metric: &metricToFetch,
				Period: &metricQueryPeriod,
				Stat:   aws.String(stat),
			},
			Id: aws.String(fmt.Sprintf("%s_%s", strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(metricName, "-", "_"), ".", "_")), stat)),
		}
	}

	endTime := time.Now()
	startTime := subtractMinutes(endTime, 10)
	getMetricDataInput := cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricDataQueries,
	}

	log.Printf("Extended metric data input: namespace %v, name %v, extended stats %v, period %v",
		namespace, metricName, extendedStats, metricQueryPeriod)

	output, err := awsservice.CwmClient.GetMetricData(context.Background(), &getMetricDataInput)
	if err != nil {
		return nil, fmt.Errorf("Error getting extended metric data %v", err)
	}

	results := make(map[string]MetricValues)
	for i, result := range output.MetricDataResults {
		stat := extendedStats[i]
		results[stat] = result.Values
		log.Printf("Extended metric values for %s: %v", stat, result.Values)
	}

	return results, nil
}
