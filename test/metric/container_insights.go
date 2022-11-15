// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type ContainerInsightsValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*ContainerInsightsValueFetcher)(nil)

func (f *ContainerInsightsValueFetcher) Fetch(namespace, metricName string, stat Statistics) ([]float64, error) {
	dimensions := f.getMetricSpecificDimensions()
	log.Print("Dimensions for container insights fetch")
	log.Print(dimensions)
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %v: %v", metricName, err.Error())
	}
	return values, err
}

var containerInsightsSupportedMetricValues = map[string]struct{}{
	"instance_memory_utilization":       {},
	"instance_number_of_running_tasks":  {},
	"instance_memory_reserved_capacity": {},
	"instance_filesystem_utilization":   {},
	"instance_network_total_bytes":      {},
	"instance_cpu_utilization":          {},
	"instance_cpu_reserved_capacity":    {},
}

func (f *ContainerInsightsValueFetcher) isApplicable(metricName string) bool {
	_, exists := containerInsightsSupportedMetricValues[metricName]
	return exists
}

var containerInsightsMetricsSpecificDimension = []types.Dimension{
	{
		Name:  aws.String("ClusterName"),
		Value: aws.String("cpu-total"),
	},
	{
		Name:  aws.String("ContainerInstanceId"),
		Value: aws.String("cpu-total"),
	},
	{
		Name:  aws.String("InstanceId"),
		Value: aws.String("cpu-total"),
	},
}

func (f *ContainerInsightsValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	//TODO currently assuming there's only one container
	containerInstances, err := test.GetContainerInstances(&(f.Env.EcsClusterArn))
	if err != nil {
		return []types.Dimension{}
	}
	log.Print("containerInstances fetch")
	log.Print(containerInstances)
	log.Print(f.Env)

	return []types.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(f.Env.EcsClusterName),
		},
		{
			Name:  aws.String("ContainerInstanceId"),
			Value: aws.String(containerInstances[0].ContainerInstanceId),
		},
		{
			Name:  aws.String("InstanceId"),
			Value: aws.String(containerInstances[0].EC2InstanceId),
		},
	}
}
