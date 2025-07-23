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

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/util"
)

type MetricPluginBoundValue map[string]map[string]map[string]float64

// Configuration constants
const (
	// Default error bound percentage (30%)
	metricErrorBound = 0.3

	// Minimum acceptable sample count deviation
	linuxSampleCountDeviation   = 15
	windowsSampleCountDeviation = 5

	// Sleep time for metric availability (in seconds)
	metricAvailabilitySleepTime = 120
)

// Todo:
// * Create a database to store these metrics instead of using cache?
// * Create a workflow to update the bound metrics?
// * Add more metrics (healthcheckextension to detect dropping metrics and prometheus to detect agent crash in EKS env)
var (
	metricPluginBoundValue = MetricPluginBoundValue{
		"1000": {
			"statsd": {
				"procstat_cpu_usage":   float64(25),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(83000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(105000),
				"net_packets_sent":     float64(105),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(20),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(82000000),
				"procstat_num_fds":     float64(11),
				"net_bytes_sent":       float64(102000),
				"net_packets_sent":     float64(105),
			},
			"logs": {
				"procstat_cpu_usage":   float64(250),
				"procstat_memory_rss":  float64(300000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(260000000),
				"procstat_num_fds":     float64(110),
				"net_bytes_sent":       float64(1800000),
				"net_packets_sent":     float64(5000),
			},
			"system": {
				"procstat_cpu_usage":   float64(16),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(35),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"prometheus": {
				"procstat_cpu_usage":   float64(12),
				"procstat_memory_rss":  float64(125000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1410000000),
				"procstat_memory_data": float64(82000000),
				"procstat_num_fds":     float64(10),
			},
		},
		"5000": {
			"statsd": {
				"procstat_cpu_usage":   float64(110),
				"procstat_memory_rss":  float64(185000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(160000000),
				"procstat_num_fds":     float64(15),
				"net_bytes_sent":       float64(524000),
				"net_packets_sent":     float64(520),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(90),
				"procstat_memory_rss":  float64(150000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(135000000),
				"procstat_num_fds":     float64(17),
				"net_bytes_sent":       float64(490000),
				"net_packets_sent":     float64(450),
			},
			"logs": {
				"procstat_cpu_usage":   float64(400),
				"procstat_memory_rss":  float64(540000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(540000000),
				"procstat_num_fds":     float64(180),
				"net_bytes_sent":       float64(6500000),
				"net_packets_sent":     float64(8500),
			},
			"system": {
				"procstat_cpu_usage":   float64(20),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(120),
			},
			"emf": {
				"procstat_cpu_usage":   float64(25),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(79000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(120),
			},
			"prometheus": {
				"procstat_cpu_usage":   float64(13),
				"procstat_memory_rss":  float64(130000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1400000000),
				"procstat_memory_data": float64(100000000),
				"procstat_num_fds":     float64(10),
			},
		},
		"10000": {
			"statsd": {
				"procstat_cpu_usage":   float64(200),
				"procstat_memory_rss":  float64(190000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(210000000),
				"procstat_num_fds":     float64(17),
				"net_bytes_sent":       float64(980000),
				"net_packets_sent":     float64(860),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(120),
				"procstat_memory_rss":  float64(130000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(165000000),
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
				"procstat_cpu_usage":   float64(35),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(120),
			},
			"emf": {
				"procstat_cpu_usage":   float64(45),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1350000000),
				"procstat_memory_data": float64(88000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(110000),
				"net_packets_sent":     float64(120),
			},
			"prometheus": {
				"procstat_cpu_usage":   float64(100),
				"procstat_memory_rss":  float64(1410000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1550000000),
				"procstat_memory_data": float64(240000000),
				"procstat_num_fds":     float64(10),
			},
		},
		// Single use case where most of the metrics will be dropped. Since the default buffer for telegraf is 10000
		// https://github.com/aws/amazon-cloudwatch-agent/blob/c85501042b088014ec40b636a8b6b2ccc9739738/translator/translate/agent/ruleMetricBufferLimit.go#L14
		// For more information on Metric Buffer and how they will exchange for the resources, please follow
		// https://github.com/influxdata/telegraf/wiki/MetricBuffer

		"50000": {
			"statsd": {
				"procstat_cpu_usage":   float64(300),
				"procstat_memory_rss":  float64(515000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(2000000000),
				"procstat_memory_data": float64(600000000),
				"procstat_num_fds":     float64(18),
				"net_bytes_sent":       float64(1700000),
				"net_packets_sent":     float64(1400),
			},
			"collectd": {
				"procstat_cpu_usage":   float64(220),
				"procstat_memory_rss":  float64(218000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1300000000),
				"procstat_memory_data": float64(240000000),
				"procstat_num_fds":     float64(18),
				"net_bytes_sent":       float64(1250000),
				"net_packets_sent":     float64(1100),
			},
			"logs": {
				"procstat_cpu_usage":   float64(400),
				"procstat_memory_rss":  float64(800000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1950000000),
				"procstat_memory_data": float64(650000000),
				"procstat_num_fds":     float64(200),
				"net_bytes_sent":       float64(6900000),
				"net_packets_sent":     float64(6500),
			},
			"system": {
				"procstat_cpu_usage":   float64(35),
				"procstat_memory_rss":  float64(110000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1200000000),
				"procstat_memory_data": float64(75000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(90000),
				"net_packets_sent":     float64(100),
			},
			"emf": {
				"procstat_cpu_usage":   float64(165),
				"procstat_memory_rss":  float64(160000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1200000000),
				"procstat_memory_data": float64(110000000),
				"procstat_num_fds":     float64(12),
				"net_bytes_sent":       float64(280000),
				"net_packets_sent":     float64(250),
			},
			"prometheus": {
				"procstat_cpu_usage":   float64(130),
				"procstat_memory_rss":  float64(360000000),
				"procstat_memory_swap": float64(0),
				"procstat_memory_vms":  float64(1700000000),
				"procstat_memory_data": float64(330000000),
				"procstat_num_fds":     float64(10),
			},
		},
	}

	windowsMetricPluginBoundValue = MetricPluginBoundValue{
		"1000": {
			"logs": {
				"procstat cpu_usage":   float64(250),
				"procstat memory_rss":  float64(220000000),
				"procstat memory_vms":  float64(888000000),
				"Bytes_Sent_Per_Sec":   float64(1800000),
				"Packets_Sent_Per_Sec": float64(5000),
			},
			"system": {
				"procstat cpu_usage":   float64(35),
				"procstat memory_rss":  float64(90000000),
				"procstat memory_vms":  float64(818000000),
				"Bytes_Sent_Per_Sec":   float64(140000),
				"Packets_Sent_Per_Sec": float64(130),
			},
		},
		"5000": {
			"logs": {
				"procstat cpu_usage":   float64(400),
				"procstat memory_rss":  float64(540000000),
				"procstat memory_vms":  float64(1100000000),
				"Bytes_Sent_Per_Sec":   float64(6500000),
				"Packets_Sent_Per_Sec": float64(8500),
			},
			"system": {
				"procstat cpu_usage":   float64(35),
				"procstat memory_rss":  float64(90000000),
				"procstat memory_vms":  float64(818000000),
				"Bytes_Sent_Per_Sec":   float64(140000),
				"Packets_Sent_Per_Sec": float64(130),
			},
		},
		"10000": {
			"logs": {
				"procstat cpu_usage":   float64(400),
				"procstat memory_rss":  float64(800000000),
				"procstat memory_vms":  float64(1500000000),
				"Bytes_Sent_Per_Sec":   float64(6820000),
				"Packets_Sent_Per_Sec": float64(8300),
			},
			"system": {
				"procstat cpu_usage":   float64(35),
				"procstat memory_rss":  float64(90000000),
				"procstat memory_vms":  float64(818000000),
				"Bytes_Sent_Per_Sec":   float64(140000),
				"Packets_Sent_Per_Sec": float64(130),
			},
		},
		// Single use case where most of the metrics will be dropped. Since the default buffer for telegraf is 10000
		// https://github.com/aws/amazon-cloudwatch-agent/blob/c85501042b088014ec40b636a8b6b2ccc9739738/translator/translate/agent/ruleMetricBufferLimit.go#L14
		// For more information on Metric Buffer and how they will exchange for the resources, please follow
		// https://github.com/influxdata/telegraf/wiki/MetricBuffer

		"50000": {
			"logs": {
				"procstat cpu_usage":   float64(400),
				"procstat memory_rss":  float64(800000000),
				"procstat memory_vms":  float64(1500000000),
				"Bytes_Sent_Per_Sec":   float64(6900000),
				"Packets_Sent_Per_Sec": float64(6500),
			},
			"system": {
				"procstat cpu_usage":   float64(35),
				"procstat memory_rss":  float64(90000000),
				"procstat memory_vms":  float64(818000000),
				"Bytes_Sent_Per_Sec":   float64(140000),
				"Packets_Sent_Per_Sec": float64(130),
			},
		},
	}
)

// StressValidator implements the stress validation logic
type StressValidator struct {
	vConfig models.ValidateConfig
	models.ValidatorFactory
}

var _ models.ValidatorFactory = (*StressValidator)(nil)

// NewStressValidator creates a new stress validator
func NewStressValidator2(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &StressValidator{
		vConfig:          vConfig,
		ValidatorFactory: basic.NewBasicValidator(vConfig),
	}
}

// CheckData validates metrics within the given time range
func (s *StressValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr         error
		ec2InstanceId    = awsservice.GetInstanceId()
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
		testCase         = s.vConfig.GetTestCase()
		validateType     = s.vConfig.GetValidateType()
	)

	log.Printf("=== STRESS TEST VALIDATION STARTED ===")
	log.Printf("Test case: %s | Validate type: %s", testCase, validateType)
	log.Printf("Time range: %v to %v", startTime, endTime)

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

		var err error
		if s.vConfig.GetOSFamily() == "windows" {
			err = s.ValidateStressMetricWindows(metric.MetricName, metricNamespace, metricDimensions, metric.MetricSampleCount, startTime, endTime)
		} else {
			err = s.ValidateStressMetric(metric.MetricName, metricNamespace, metricDimensions, metric.MetricSampleCount, startTime, endTime)
		}
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	if multiErr != nil {
		log.Printf("=== STRESS TEST VALIDATION FAILED ===")
		log.Printf("Test case: %s, validate type: %s, error:\n %+v", testCase, validateType, multiErr)
	} else {
		log.Printf("=== STRESS TEST VALIDATION SUCCEEDED ===")
		log.Printf("Test case: %s, validate type: %s completed successfully", testCase, validateType)
	}

	return multiErr
}

// ValidateStressMetric validates a single metric for Linux
func (s *StressValidator) ValidateStressMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		dataRate       = fmt.Sprint(s.vConfig.GetDataRate())
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
		receiver       = s.vConfig.GetPluginsConfig()[0] //Assuming one plugin at a time
	)

	log.Printf("--- VALIDATING METRIC: %s ---", metricName)
	log.Printf("Namespace: %s | Start: %v | End: %v", metricNamespace, startTime, endTime)

	// Check if plugin and metric bounds are configured
	if _, ok := metricPluginBoundValue[dataRate]; !ok {
		return fmt.Errorf("configuration error: data rate '%s' not found in metric bounds", dataRate)
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver]; !ok {
		return fmt.Errorf("configuration error: plugin '%s' not found for data rate '%s'", receiver, dataRate)
	}

	if _, ok := metricPluginBoundValue[dataRate][receiver][metricName]; !ok {
		return fmt.Errorf("configuration error: metric '%s' not found for plugin '%s' at data rate '%s'",
			metricName, receiver, dataRate)
	}

	// Build and execute metric queries
	stressMetricQueries := s.buildStressMetricQueries(metricName, metricNamespace, metricDimensions)
	metrics, err := awsservice.GetMetricData(stressMetricQueries, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get metric data for %s: %w", metricName, err)
	}

	// Validate metric data exists
	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("no data found for metric %s with namespace %s and dimensions %v",
			metricName, metricNamespace, util.LogCloudWatchDimension(metricDimensions))
	}

	// Validate metric value against bounds
	metricValue := metrics.MetricDataResults[0].Values[0]
	upperBoundValue := metricPluginBoundValue[dataRate][receiver][metricName] * (1 + metricErrorBound)
	log.Printf("METRIC VALUE: %s = %.2f | Upper Bound: %.2f", metricName, metricValue, upperBoundValue)

	if metricValue < 0 || metricValue > upperBoundValue {
		return fmt.Errorf("validation failed: metric '%s' value %.2f exceeds limit %.2f",
			metricName, metricValue, upperBoundValue)
	}

	// Validate sample count
	minSampleCount := metricSampleCount - linuxSampleCountDeviation
	log.Printf("SAMPLE COUNT: Expected range [%d, %d] for metric '%s'",
		minSampleCount, metricSampleCount, metricName)

	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions,
		startTime, endTime, minSampleCount, metricSampleCount, int32(boundAndPeriod)); !ok {
		return fmt.Errorf("validation failed: metric '%s' sample count not within expected range [%d, %d]",
			metricName, minSampleCount, metricSampleCount)
	}

	log.Printf("VALIDATION PASSED: Metric '%s' within acceptable limits", metricName)
	return nil
}

// ValidateStressMetricWindows validates a single metric for Windows
func (s *StressValidator) ValidateStressMetricWindows(metricName, metricNamespace string, metricDimensions []types.Dimension, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		dataRate       = fmt.Sprint(s.vConfig.GetDataRate())
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
		receiver       = s.vConfig.GetPluginsConfig()[0] //Assuming one plugin at a time
	)

	log.Printf("--- VALIDATING WINDOWS METRIC: %s ---", metricName)
	log.Printf("Namespace: %s | Start: %v | End: %v", metricNamespace, startTime, endTime)

	// Check if plugin and metric bounds are configured
	if _, ok := windowsMetricPluginBoundValue[dataRate]; !ok {
		return fmt.Errorf("configuration error: data rate '%s' not found in Windows metric bounds", dataRate)
	}

	if _, ok := windowsMetricPluginBoundValue[dataRate][receiver]; !ok {
		return fmt.Errorf("configuration error: plugin '%s' not found for data rate '%s' in Windows",
			receiver, dataRate)
	}

	if _, ok := windowsMetricPluginBoundValue[dataRate][receiver][metricName]; !ok {
		return fmt.Errorf("configuration error: metric '%s' not found for plugin '%s' at data rate '%s' in Windows",
			metricName, receiver, dataRate)
	}

	// Get metric statistics
	metrics, err := awsservice.GetMetricStatistics(
		metricName,
		metricNamespace,
		metricDimensions,
		startTime,
		endTime,
		int32(boundAndPeriod),
		[]types.Statistic{types.StatisticMaximum},
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to get metric statistics for %s: %w", metricName, err)
	}

	// Validate metric data exists
	if len(metrics.Datapoints) == 0 || metrics.Datapoints[0].Maximum == nil {
		return fmt.Errorf("no data found for metric %s with namespace %s and dimensions %v",
			metricName, metricNamespace, util.LogCloudWatchDimension(metricDimensions))
	}

	// Validate metric value against bounds
	metricValue := *metrics.Datapoints[0].Maximum
	upperBoundValue := windowsMetricPluginBoundValue[dataRate][receiver][metricName] * (1 + metricErrorBound)
	log.Printf("METRIC VALUE: %s = %.2f | Upper Bound: %.2f", metricName, metricValue, upperBoundValue)

	if metricValue < 0 || metricValue > upperBoundValue {
		return fmt.Errorf("validation failed: metric '%s' value %.2f exceeds limit %.2f",
			metricName, metricValue, upperBoundValue)
	}

	// Validate sample count
	minSampleCount := metricSampleCount - windowsSampleCountDeviation
	log.Printf("SAMPLE COUNT: Expected range [%d, %d] for metric '%s'",
		minSampleCount, metricSampleCount, metricName)

	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions,
		startTime, endTime, minSampleCount, metricSampleCount, int32(boundAndPeriod)); !ok {
		return fmt.Errorf("validation failed: metric '%s' sample count not within expected range [%d, %d]",
			metricName, minSampleCount, metricSampleCount)
	}

	log.Printf("VALIDATION PASSED: Metric '%s' within acceptable limits", metricName)
	return nil
}

// buildStressMetricQueries creates the metric data queries
func (s *StressValidator) buildStressMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) []types.MetricDataQuery {
	metricQueryPeriod := int32(s.vConfig.GetAgentCollectionPeriod().Seconds())

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
