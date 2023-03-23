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
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
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
				"procstat_cpu_usage":   float64(40),
				"procstat_memory_rss":  float64(152000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(888000000),
				"procstat_memory_data": float64(162000000),
				"procstat_num_fds":     float64(110),
				"net_bytes_sent":       float64(170000),
				"net_packets_sent":     float64(1500),
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
				"procstat_cpu_usage":   float64(200),
				"procstat_memory_rss":  float64(325000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1100000000),
				"procstat_memory_data": float64(400000000),
				"procstat_num_fds":     float64(111),
				"net_bytes_sent":       float64(6500000),
				"net_packets_sent":     float64(8500),
			},
		},
		"10000": {
			"statsd": {
				"procstat_cpu_usage":   float64(135),
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
				"procstat_cpu_usage":   float64(225),
				"procstat_memory_rss":  float64(440000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1200000000),
				"procstat_memory_data": float64(440000000),
				"procstat_num_fds":     float64(130),
				"net_bytes_sent":       float64(6820000),
				"net_packets_sent":     float64(8300),
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
				"procstat_cpu_usage":   float64(200),
				"procstat_memory_rss":  float64(600000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1400000000),
				"procstat_memory_data": float64(650000000),
				"procstat_num_fds":     float64(125),
				"net_bytes_sent":       float64(6900000),
				"net_packets_sent":     float64(6500),
			},
		},
	}
)

type StressValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*StressValidator)(nil)

func NewStressValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &StressValidator{
		vConfig: vConfig,
	}
}

func (s *StressValidator) GenerateLoad() (err error) {
	var (
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		dataType              = s.vConfig.GetDataType()
		dataRate              = s.vConfig.GetDataRate()
		receiver              = s.vConfig.GetPluginsConfig()
	)
	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, dataRate)
	default:
		// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
		err = common.StartSendingMetrics(receiver, agentCollectionPeriod, dataRate)
	}

	return err
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
		err := s.ValidateStressMetric(metric.MetricName, metricNamespace, metricDimensions, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *StressValidator) Cleanup() error {
	var (
		dataType      = s.vConfig.GetDataType()
		ec2InstanceId = awsservice.GetInstanceId()
	)
	switch dataType {
	case "logs":
		awsservice.DeleteLogGroup(ec2InstanceId)
	}

	return nil
}

func (s *StressValidator) ValidateStressMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, startTime, endTime time.Time) error {
	var (
		dataRate       = fmt.Sprint(s.vConfig.GetDataRate())
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
		receiver       = s.vConfig.GetPluginsConfig()
	)

	stressMetricQueries := s.buildStressMetricQueries(metricName, metricNamespace, metricDimensions)

	log.Printf("Start to collect and validate metric %s with the namespace %s, start time %v and end time %v", metricName, metricNamespace, startTime, endTime)

	// We are only interesting in the maxium metric values within the time range
	metrics, err := awsservice.GetMetricData(stressMetricQueries, startTime, endTime)
	if err != nil {
		return err
	}

	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, metricDimensions)
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver]; !ok {
		return fmt.Errorf("plugin %s does not have data rate", receiver)
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver][metricName]; !ok {
		return fmt.Errorf("metric %s does not have bound", receiver)
	}

	// Assuming each plugin are testing one at a time
	// Validate if the corresponding metrics are within the acceptable range [acceptable value +- 30%]
	metricValue := metrics.MetricDataResults[0].Values[0]
	upperBoundValue := metricPluginBoundValue[dataRate][receiver][metricName] * (1 + metricErrorBound)
	log.Printf("Metric %s within the namespace %s has value of %f and the upper bound is %f", metricName, metricNamespace, metricValue, upperBoundValue)

	if metricValue < 0 || metricValue > upperBoundValue {
		return fmt.Errorf("metric %s with value %f is larger than %f limit", metricName, metricValue, upperBoundValue)
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)
	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, int(boundAndPeriod-5), int(boundAndPeriod), int32(boundAndPeriod)); !ok {
		return fmt.Errorf("metric %s is not within sample count bound [ %f, %f]", metricName, boundAndPeriod-5, boundAndPeriod)
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
