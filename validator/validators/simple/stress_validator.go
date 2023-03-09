// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package simple

import (
	"time"

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
				"procstat_cpu_usage":   float64(19),
				"procstat_memory_rss":  float64(66500000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(95000000),
				"procstat_num_fds":     float64(9),
				"net_bytes_sent":       float64(100000),
				"net_packets_sent":     float64(100),
			},
		},
		"5000": {
			"statsd": {
				"procstat_cpu_usage":   float64(120),
				"procstat_memory_rss":  float64(130000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(130000000),
				"procstat_num_fds":     float64(19),
				"net_bytes_sent":       float64(524000),
				"net_packets_sent":     float64(520),
			},
		},
		"10000": {
			"statsd": {
				"procstat_cpu_usage":   float64(200),
				"procstat_memory_rss":  float64(160000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(818000000),
				"procstat_memory_data": float64(177000000),
				"procstat_num_fds":     float64(19),
				"net_bytes_sent":       float64(980000),
				"net_packets_sent":     float64(860),
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
				"net_packets_sent":     float64(10400),
			},
		},
	}
)

type SimpleValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*SimpleValidator)(nil)

func NewSimpleValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &SimpleValidator{
		vConfig: vConfig,
	}
}

func (s *SimpleValidator) GenerateLoad() (err error) {
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

func (s *SimpleValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr error
	)

	return multiErr
}

func (s *SimpleValidator) Cleanup() error {
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
