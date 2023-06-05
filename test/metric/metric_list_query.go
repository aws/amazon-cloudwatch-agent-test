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

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
)

type MetricListFetcher struct {
}

func (n *MetricListFetcher) Fetch(namespace, metricName string, dimensions []types.Dimension) ([]types.Metric, error) {
	var dims []types.DimensionFilter
	for _, dim := range dimensions {
		dims = append(dims, types.DimensionFilter{
			Name:  dim.Name,
			Value: dim.Value,
		})
	}

	listMetricInput := cloudwatch.ListMetricsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: dims,
	}

	log.Printf("Metric data input: namespace %v, name %v", namespace, metricName)

	output, err := awsservice.CwmClient.ListMetrics(context.Background(), &listMetricInput)
	if err != nil {
		return nil, fmt.Errorf("Error getting metric data %v", err)
	}

	log.Printf("Metrics fetched : %s", fmt.Sprint(output))

	return output.Metrics, nil
}
