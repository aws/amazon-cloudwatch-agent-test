// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

var metricDefaultDimensionFactories = []MetricDefaultDimensionFactory{
	&CPUMetricDimension{},
	&MemMetricDimension{},
	&ProcStatMetricDimension{},
	&DiskIOMetricDimension{},
	&NetMetricDimension{},
	&ContainerInsightsMetricDimension{},
}

func GetMetricDefaultDimensions(env environment.MetaData, metricName string) []types.Dimension {
	for _, dimension := range metricDefaultDimensionFactories {
		if dimension.isApplicable(metricName) {
			return dimension.getMetricSpecificDimensions(env)
		}
	}
	return []types.Dimension{}
}

type MetricDefaultDimensionFactory interface {
	isApplicable(string) bool
	getMetricSpecificDimensions(env environment.MetaData) []types.Dimension
}
