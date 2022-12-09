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

type StatsdMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*StatsdMetricValueFetcher)(nil)

func (f *StatsdMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions()
	dimensions = append(dimensions, f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %v: %v", metricName, err.Error())
	}
	return values, err
}

func (f *StatsdMetricValueFetcher) isApplicable(metricName string) bool {
	statsdSupportedMetric := f.getPluginSupportedMetric()
	_, exists := statsdSupportedMetric[metricName]
	return exists
}

func (f *StatsdMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// EC2 Image Builder creates a bash script that sends statsd format to cwagent at port 8125
	// The bash script is at /etc/statsd.sh
	//    for times in  {1..3}
	//    do
	//      echo "statsd.counter:1|c" | nc -w 1 -u 127.0.0.1 8125
	//      sleep 60
	//    done
	return map[string]struct{}{
		"statsd_counter": {},
	}
}
func (f *StatsdMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("metric_type"),
			Value: aws.String("counter"),
		},
	}
}
