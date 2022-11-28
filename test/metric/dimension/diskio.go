// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type DiskIOMetricDimension struct {
}

var _ MetricDefaultDimensionFactory = (*DiskIOMetricDimension)(nil)

func (f *DiskIOMetricDimension) isApplicable(metricName string) bool {
	diskIOSupportedMetric := f.getPluginSupportedMetric()
	_, exists := diskIOSupportedMetric[metricName]
	return exists
}

func (f *DiskIOMetricDimension) getPluginSupportedMetric() map[string]struct{} {
	// DiskIO Supported Metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L12
	return map[string]struct{}{
		"diskio_iops_in_progress": {},
		"diskio_io_time":          {},
		"diskio_reads":            {},
		"diskio_read_bytes":       {},
		"diskio_read_time":        {},
		"diskio_writes":           {},
		"diskio_write_bytes":      {},
		"diskio_write_time":       {},
	}
}

func (f *DiskIOMetricDimension) getMetricSpecificDimensions(env environment.MetaData) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("name"),
			Value: aws.String("nvme0n1"),
		},
	}
}
