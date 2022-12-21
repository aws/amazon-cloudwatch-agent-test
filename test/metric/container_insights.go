// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux
// +build linux

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
)

type ContainerInsightsValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*ContainerInsightsValueFetcher)(nil)

func (f *ContainerInsightsValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions()
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *ContainerInsightsValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	return map[string]struct{}{
		"instance_memory_utilization":       {},
		"instance_number_of_running_tasks":  {},
		"instance_memory_reserved_capacity": {},
		"instance_filesystem_utilization":   {},
		"instance_network_total_bytes":      {},
		"instance_cpu_utilization":          {},
		"instance_cpu_reserved_capacity":    {},
	}
}

func (f *ContainerInsightsValueFetcher) isApplicable(metricName string) bool {
	_, exists := f.getPluginSupportedMetric()[metricName]
	return exists
}

func (f *ContainerInsightsValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	//TODO currently assuming there's only one container
	containerInstances, err := awsservice.GetContainerInstances(f.getEnv().EcsClusterArn)
	if err != nil {
		log.Print(err)
		return []types.Dimension{}
	}

	return []types.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(f.getEnv().EcsClusterName),
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
