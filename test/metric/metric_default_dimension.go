// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func GetMetricDefaultDimensions(metricName string) map[string]struct{} {
	dimensions, ok := metric_to_default_dimension_map[metricName]
	if !ok {
		return []types.Dimension{}
	}
	return dimensions
}

const metric_to_default_dimension_map = map[string][]types.Dimension{
	// CPU supported metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L9-L10
	"cpu_time_active":      cpu_default_dimension,
	"cpu_time_guest":       cpu_default_dimension,
	"cpu_time_guest_nice":  cpu_default_dimension,
	"cpu_time_idle":        cpu_default_dimension,
	"cpu_time_irq":         cpu_default_dimension,
	"cpu_time_nice":        cpu_default_dimension,
	"cpu_time_softirq":     cpu_default_dimension,
	"cpu_time_system":      cpu_default_dimension,
	"cpu_time_user":        cpu_default_dimension,
	"cpu_usage_active":     cpu_default_dimension,
	"cpu_usage_guest":      cpu_default_dimension,
	"cpu_usage_guest_nice": cpu_default_dimension,
	"cpu_usage_idle":       cpu_default_dimension,
	"cpu_usage_iowait":     cpu_default_dimension,
	"cpu_usage_irq":        cpu_default_dimension,
	"cpu_usage_nice":       cpu_default_dimension,
	"cpu_usage_softirq":    cpu_default_dimension,
	"cpu_usage_steal":      cpu_default_dimension,
	"cpu_usage_system":     cpu_default_dimension,
	"cpu_usage_user":       cpu_default_dimension,
}

const cpu_default_dimension = []types.Dimension{
	{
		Name:  aws.String("cpu"),
		Value: aws.String("cpu-total"),
	},
}
