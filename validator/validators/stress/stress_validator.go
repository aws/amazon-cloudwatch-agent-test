// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package stress

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/util"
)

type MetricPluginBoundValue map[string]map[string]map[string]float64

// Todo:
// * Create a database  to store these metrics instead of using cache?
// * Create a workflow to update the bound metrics?
// * Add more metrics (healthcheckextension to detect dropping metrics and prometheus to detect agent crash in EKS env)
var (
	metricErrorBound       = 0.3
	metricPluginBoundValue = MetricPluginBoundValue{
		"1000": {
			"statsd": {
				"procstat_cpu_usage":   float64(25),
				"procstat_memory_rss":  float64(82000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(83000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(105000),
				"net_packets_sent":     float64(105),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(20),
				"procstat_memory_rss":  float64(80000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(82000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(102000),
				"net_packets_sent":     float64(105),
			},
			"logs": {
				"procstat_cpu_usage":   float64(250),
				"procstat_memory_rss":  float64(220000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(260000000),
				"procstat_num_fds":     float64(110),
				"net_bytes_sent":       float64(1800000),
				"net_packets_sent":     float64(5000),
			},
			"system": {
				"procstat_cpu_usage":   float64(15),
				"procstat_memory_rss":  float64(75000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(15),
				"procstat_memory_rss":  float64(75000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
		},
		"5000": {
			"statsd": {
				"procstat_cpu_usage":   float64(100),
				"procstat_memory_rss":  float64(130000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(135000000),
				"procstat_num_fds":     float64(15),
				"net_bytes_sent":       float64(524000),
				"net_packets_sent":     float64(520),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(90),
				"procstat_memory_rss":  float64(120000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(135000000),
				"procstat_num_fds":     float64(17),
				"net_bytes_sent":       float64(490000),
				"net_packets_sent":     float64(450),
			},
			"logs": {
				"procstat_cpu_usage":   float64(400),
				"procstat_memory_rss":  float64(540000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1100000000),
				"procstat_memory_data": float64(540000000),
				"procstat_num_fds":     float64(180),
				"net_bytes_sent":       float64(6500000),
				"net_packets_sent":     float64(8500),
			},
			"system": {
				"procstat_cpu_usage":   float64(15),
				"procstat_memory_rss":  float64(75000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(25),
				"procstat_memory_rss":  float64(80000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(79000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(120),
			},
		},
		"10000": {
			"statsd": {
				"procstat_cpu_usage":   float64(150),
				"procstat_memory_rss":  float64(160000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(177000000),
				"procstat_num_fds":     float64(17),
				"net_bytes_sent":       float64(980000),
				"net_packets_sent":     float64(860),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(120),
				"procstat_memory_rss":  float64(130000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(150000000),
				"procstat_num_fds":     float64(17),
				"net_bytes_sent":       float64(760000),
				"net_packets_sent":     float64(700),
			},
			"logs": {
				"procstat_cpu_usage":   float64(400),
				"procstat_memory_rss":  float64(800000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1500000000),
				"procstat_memory_data": float64(840000000),
				"procstat_num_fds":     float64(180),
				"net_bytes_sent":       float64(6820000),
				"net_packets_sent":     float64(8300),
			},
			"system": {
				"procstat_cpu_usage":   float64(15),
				"procstat_memory_rss":  float64(75000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(40),
				"procstat_memory_rss":  float64(85000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(88000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(120),
			},
		},
		// Single use case where most of the metrics will be dropped. Since the default buffer for telegraf is 10000
		// https://github.com/aws/amazon-cloudwatch-agent/blob/c85501042b088014ec40b636a8b6b2ccc9739738/translator/translate/agent/ruleMetricBufferLimit.go#L14
		// For more information on Metric Buffer and how they will exchange for the resources, please follow
		// https://github.com/influxdata/telegraf/wiki/MetricBuffer

		"50000": {
			"statsd": {
				"procstat_cpu_usage":   float64(250),
				"procstat_memory_rss":  float64(300000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1000000000),
				"procstat_memory_data": float64(330000000),
				"procstat_num_fds":     float64(18),
				"net_bytes_sent":       float64(1700000),
				"net_packets_sent":     float64(1400),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(220),
				"procstat_memory_rss":  float64(218000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(980000000),
				"procstat_memory_data": float64(240000000),
				"procstat_num_fds":     float64(18),
				"net_bytes_sent":       float64(1250000),
				"net_packets_sent":     float64(1100),
			},
			"logs": {
				"procstat_cpu_usage":   float64(400),
				"procstat_memory_rss":  float64(800000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1500000000),
				"procstat_memory_data": float64(650000000),
				"procstat_num_fds":     float64(200),
				"net_bytes_sent":       float64(6900000),
				"net_packets_sent":     float64(6500),
			},
			"system": {
				"procstat_cpu_usage":   float64(15),
				"procstat_memory_rss":  float64(75000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(165),
				"procstat_memory_rss":  float64(100000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(110000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(280000),
				"net_packets_sent":     float64(220),
			},
		},
	}
)

type StressValidator struct {
	vConfig models.ValidateConfig
	models.ValidatorFactory
}

var _ models.ValidatorFactory = (*StressValidator)(nil)

func NewStressValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &StressValidator{
		vConfig:          vConfig,
		ValidatorFactory: basic.NewBasicValidator(vConfig),
	}
}

func (s *StressValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr         error
		ec2InstanceId    = awsservice.GetInstanceId()
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
	)
	for _, metric := range validationMetric {
		metricDimensions := []types.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(ec2InstanceId),
			},
		}
		for _, dimension := range metric.MetricDimension {
			metricDimensions = append(metricDimensions, types.Dimension{
				Name:  aws.String(dimension.Name),
				Value: aws.String(dimension.Value),
			})
		}
		err := s.ValidateStressMetric(metric.MetricName, metricNamespace, metricDimensions, metric.MetricSampleCount, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *StressValidator) ValidateStressMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		dataRate       = fmt.Sprint(s.vConfig.GetDataRate())
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
		receiver       = s.vConfig.GetPluginsConfig()[0] //Assuming one plugin at a time
	)

	stressMetricQueries := s.buildStressMetricQueries(metricName, metricNamespace, metricDimensions)

	log.Printf("Start to collect and validate metric %s with the namespace %s, start time %v and end time %v \n", metricName, metricNamespace, startTime, endTime)

	// We are only interesting in the maxium metric values within the time range
	metrics, err := awsservice.GetMetricData(stressMetricQueries, startTime, endTime)
	if err != nil {
		return err
	}

	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("\n getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, util.LogCloudWatchDimension(metricDimensions))
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver]; !ok {
		return fmt.Errorf("\n plugin %s does not have data rate", receiver)
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver][metricName]; !ok {
		return fmt.Errorf("\n metric %s does not have bound", receiver)
	}

	// Assuming each plugin are testing one at a time
	// Validate if the corresponding metrics are within the acceptable range [acceptable value +- 30%]
	metricValue := metrics.MetricDataResults[0].Values[0]
	upperBoundValue := metricPluginBoundValue[dataRate][receiver][metricName] * (1 + metricErrorBound)
	log.Printf("Metric %s within the namespace %s has value of %f and the upper bound is %f \n", metricName, metricNamespace, metricValue, upperBoundValue)

	if metricValue < 0 || metricValue > upperBoundValue {
		return fmt.Errorf("\n metric %s with value %f is larger than %f limit", metricName, metricValue, upperBoundValue)
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)
	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, metricSampleCount-5, metricSampleCount, int32(boundAndPeriod)); !ok {
		return fmt.Errorf("\n metric %s is not within sample count bound [ %d, %d]", metricName, metricSampleCount-5, metricSampleCount)
	}

	return nil
}

func (s *StressValidator) buildStressMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) []types.MetricDataQuery {
	var (
		metricQueryPeriod = int32(s.vConfig.GetAgentCollectionPeriod().Seconds())
	)

	metricInformation := types.Metric{
		Namespace:  aws.String(metricNamespace),
		MetricName: aws.String(metricName),
		Dimensions: metricDimensions,
	}

	metricDataQueries := []types.MetricDataQuery{
		{
			MetricStat: &types.MetricStat{
				Metric: &metricInformation,
				Period: &metricQueryPeriod,
				Stat:   aws.String(string(models.MAXIMUM)),
			},
			Id: aws.String(strings.ToLower(metricName)),
		},
	}
	return metricDataQueries
}
