// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type Fetcher struct {
}

func (n *Fetcher) Fetch(namespace, metricName string, dimensions []types.Dimension) ([]types.Metric, error) {
	var dims []types.DimensionFilter
	for _, dim := range dimensions {
		dims = append(dims, types.DimensionFilter{
			Name:  dim.Name,
			Value: dim.Value,
		})
	}

	listMetricInput := cloudwatch.ListMetricsInput{
		Namespace:  aws.String(namespace),
		Dimensions: dims,
	}
	if len(metricName) > 0 {
		listMetricInput.MetricName = aws.String(metricName)
	}

	dimStr := ""
	for _, dim := range dimensions {
		dimStr += fmt.Sprintf("{Name: %s, Value: %s} ", *dim.Name, *dim.Value)
	}
	log.Printf("Metric data input: namespace %v, name %v, dimensions [%v]", namespace, metricName, dimStr)


	// log.Printf("Metric data input: namespace %v, name %v", namespace, metricName)
	var metrics []types.Metric
	for {
		// get a complete list of metrics with given dimensions
		output, err := awsservice.CwmClient.ListMetrics(context.Background(), &listMetricInput)
		if err != nil {
			return nil, fmt.Errorf("Error getting metric data %v", err)
		}
		metrics = append(metrics, output.Metrics...)
		// nil or empty nextToken means there is no more data to be fetched
		nextToken := output.NextToken
		if nextToken == nil || *nextToken == "" {
			break
		}
		listMetricInput.NextToken = nextToken
	}
	log.Printf("total number of metrics fetched: %v", len(metrics))

	metricsJSON, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		log.Printf("Error marshalling metrics: %v", err)
	} else {
		log.Printf("Metric data input: %s", string(metricsJSON))
	}

	return metrics, nil
}
