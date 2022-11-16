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

type DiskIOMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*DiskIOMetricValueFetcher)(nil)

func (f *DiskIOMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dims := f.getMetricSpecificDimensions()
	dims = append(dims, f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dims, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %v", metricName, err)
	}
	return values, err
}

func (f *DiskIOMetricValueFetcher) isApplicable(metricName string) bool {
	diskIOSupportedMetric := f.getPluginSupportedMetric()
	_, exists := diskIOSupportedMetric[metricName]
	return exists
}

func (f *DiskIOMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
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

func (f *DiskIOMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("name"),
			Value: aws.String("nvme0n1"),
		},
	}
}
