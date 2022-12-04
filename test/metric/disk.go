// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type DiskMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*DiskMetricValueFetcher)(nil)

func (f *DiskMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions()
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for $s: $v", metricName, err)
	}
	return values, err
}

func (f *DiskMetricValueFetcher) isApplicable(metricName string) bool {
	diskSupportedMetric := f.getPluginSupportedMetric()
	_, exists := diskSupportedMetric[metricName]
	return exists
}

// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L11
func (f *DiskMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	return map[string]struct{}{
		"disk_free":         {},
		"disk_inodes_free":  {},
		"disk_inodes_total": {},
		"disk_inodes_used":  {},
		"disk_total":        {},
		"disk_used":         {},
		"disk_used_percent": {},
	}
}

func (f *DiskMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("test"),
			Value: aws.String("disk-sanity"),
		},
	}
}
