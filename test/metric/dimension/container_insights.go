// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type ContainerInsightsMetricDimension struct {
}

var _ MetricDefaultDimensionFactory = (*ContainerInsightsMetricDimension)(nil)

func (f *ContainerInsightsMetricDimension) getPluginSupportedMetric() map[string]struct{} {
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

func (f *ContainerInsightsMetricDimension) isApplicable(metricName string) bool {
	_, exists := f.getPluginSupportedMetric()[metricName]
	return exists
}

func (f *ContainerInsightsMetricDimension) getMetricSpecificDimensions(env environment.MetaData) []types.Dimension {
	//TODO currently assuming there's only one container
	containerInstances, err := test.GetContainerInstances(env.EcsClusterArn)
	if err != nil {
		log.Print(err)
		return []types.Dimension{}
	}

	return []types.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(env.EcsClusterName),
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
