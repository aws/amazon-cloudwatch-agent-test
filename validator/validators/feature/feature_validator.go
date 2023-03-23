// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package feature

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

type FeatureValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*FeatureValidator)(nil)

func NewFeatureValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &FeatureValidator{
		vConfig: vConfig,
	}
}

func (s *FeatureValidator) GenerateLoad() error {
	var (
		multiErr              error
		dataRate              = s.vConfig.GetDataRate()
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		receivers             = s.vConfig.GetPluginsConfig()
	)

	if err := common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, dataRate); err != nil {
		multiErr = multierr.Append(multiErr, err)
	}

	// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
	for _, receiver := range receivers {
		if err := common.StartSendingMetrics(receiver, agentCollectionPeriod, dataRate); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *FeatureValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr         error
		ec2InstanceId    = awsservice.GetInstanceId()
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
		validationLog    = s.vConfig.GetLogValidation()
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
		err := s.ValidateMetric(metric.MetricName, metricNamespace, metricDimensions, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	for _, log := range validationLog {
		err := s.ValidateLogs(log.LogValue, log.LogLines)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *FeatureValidator) Cleanup() error {
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

func (s *FeatureValidator) ValidateLogs(logStream string, logLine string, numberOfLogLine int, startTime, endTime time.Time) error {
	var (
		logGroup = awsservice.GetInstanceId()
	)
	ok, err := awsservice.ValidateLogs(logGroup, logStream, &startTime, &endTime, func(logs []string) bool {
		if len(logs) < 1 {
			return false
		}
		actualNumberOfLogLines := 0
		for _, log := range logs {
			if strings.Contains(log, logLine) {
				actualNumberOfLogLines += 1
			}
		}
		if actualNumberOfLogLines != numberOfLogLine {
			return false
		}
		return true
	})

	if !ok || err != nil {
		return fmt.Errorf("Validate logs failed")
	}
	return nil
}

func (s *FeatureValidator) ValidateMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, startTime, endTime time.Time) error {
	var boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
	// Metric being send every one minute; therefore, the sample count is only 1
	// https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/internal/common/metrics.go#L58-L72
	// and the aggregation is 1 minute too
	if strings.Contains(metricName, "statsd") || strings.Contains(metricName, "collectd") {
		boundAndPeriod = 1
	}

	stressMetricQueries := s.buildMetricQueries(metricName, metricNamespace, metricDimensions)

	log.Printf("Start to collect and validate metric %s with the namespace %s, start time %v and end time %v", metricName, metricNamespace, startTime, endTime)

	// We are only interesting in the maxium metric values within the time range
	metrics, err := awsservice.GetMetricData(stressMetricQueries, startTime, endTime)
	if err != nil {
		return err
	}

	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, metricDimensions)
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)
	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, int(boundAndPeriod), int(boundAndPeriod), int32(boundAndPeriod)); !ok {
		return fmt.Errorf("metric %s is not within sample count bound [ %f, %f]", metricName, boundAndPeriod, boundAndPeriod)
	}

	return nil
}

func (s *FeatureValidator) buildMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) []types.MetricDataQuery {
	var metricQueryPeriod = int32(s.vConfig.GetAgentCollectionPeriod().Seconds())

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
				Stat:   aws.String(string(models.AVERAGE)),
			},
			Id: aws.String(strings.ToLower(metricName)),
		},
	}
	return metricDataQueries
}
