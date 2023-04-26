// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package basic

import (
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/qri-io/jsonschema"
	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/util"
)

//go:embed resources/emf_metrics_schema.json
var EmfSchema string

const metricErrorBound = 0.1

type BasicValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*BasicValidator)(nil)

func NewBasicValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &BasicValidator{
		vConfig: vConfig,
	}
}

func (s *BasicValidator) GenerateLoad() error {
	var (
		metricSendingInterval = time.Minute
		logGroup              = awsservice.GetInstanceId()
		metricNamespace       = s.vConfig.GetMetricNamespace()
		dataRate              = s.vConfig.GetDataRate()
		dataType              = s.vConfig.GetDataType()
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		receiver              = s.vConfig.GetPluginsConfig()[0]
	)

	switch dataType {
	case "logs":
		return common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, metricSendingInterval, dataRate)
	default:
		// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
		return common.StartSendingMetrics(receiver, agentCollectionPeriod, metricSendingInterval, dataRate, logGroup, metricNamespace)
	}
}

func (s *BasicValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr         error
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
		validationLog    = s.vConfig.GetLogValidation()
		validationEMFLog = s.vConfig.GetEMFValidation()
	)

	for _, metric := range validationMetric {
		var metricDimensions []types.Dimension
		for _, dimension := range metric.MetricDimension {
			metricDimensions = append(metricDimensions, types.Dimension{
				Name:  aws.String(dimension.Name),
				Value: aws.String(dimension.Value),
			})
		}
		err := s.ValidateMetric(metric.MetricName, metricNamespace, metricDimensions, metric.MetricValue, metric.MetricSampleCount, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	for _, log := range validationLog {
		err := s.ValidateLogs(log.LogStream, log.LogValue, log.LogLines, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	for _, emfLog := range validationEMFLog {
		err := s.ValidateEMFLogs(emfLog.LogStream, emfLog.EmfSchema, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *BasicValidator) Cleanup() error {
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

func (s *BasicValidator) ValidateEMFLogs(logStream, emfSchemaName string, startTime, endTime time.Time) error {
	var (
		logGroup                            = awsservice.GetInstanceId()
		validateSchema, validateLogContents = getEMFSchemaAndValidateFunction(emfSchemaName)
	)

	log.Printf("Start to validate emf json schema %s with log group %s, log stream %s, start time %v and end time %v", emfSchemaName, logGroup, logStream, startTime, endTime)
	ok, err := awsservice.ValidateLogs(logGroup, logStream, &startTime, &endTime, func(logs []string) bool {
		if len(logs) < 1 {
			return false
		}

		for _, l := range logs {
			if !awsservice.MatchEMFLogWithSchema(l, validateSchema, validateLogContents) {
				return false
			}
		}
		return true
	})

	if !ok || err != nil {
		return fmt.Errorf("\n Failed to validate emf json schema %s with log group %s, log stream %s, start time %v and end time %v", emfSchemaName, logGroup, logStream, startTime, endTime)
	}

	return nil
}

func (s *BasicValidator) ValidateLogs(logStream, logLine string, numberOfLogLine int, startTime, endTime time.Time) error {
	var (
		logGroup = awsservice.GetInstanceId()
	)
	log.Printf("Start to validate log '%s' with number of logs lines %d within log group %s, log stream %s, start time %v and end time %v", logLine, numberOfLogLine, logGroup, logStream, startTime, endTime)
	ok, err := awsservice.ValidateLogs(logGroup, logStream, &startTime, &endTime, func(logs []string) bool {
		if len(logs) < 1 {
			return false
		}
		actualNumberOfLogLines := 0
		for _, l := range logs {
			if strings.Contains(l, logLine) {
				actualNumberOfLogLines += 1
			}
		}

		return numberOfLogLine <= actualNumberOfLogLines
	})

	if !ok || err != nil {
		return fmt.Errorf("\n the number of log line for '%s' is %d which does not match the actual number with log group %s, log stream %s, start time %v and end time %v with err %v", logLine, numberOfLogLine, logGroup, logStream, startTime, endTime, err)
	}

	return nil
}

func (s *BasicValidator) ValidateMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, metricValue float64, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		boundAndPeriod = int32(s.vConfig.GetAgentCollectionPeriod().Seconds())
	)

	log.Printf("Start to collect and validate metric %s with the dimensions %v, namespace %s, start time %v and end time %v \n", metricName, util.LogCloudWatchDimension(metricDimensions), metricNamespace, startTime, endTime)

	metrics, err := awsservice.GetMetricStatistics(metricName, metricNamespace, metricDimensions, startTime, endTime, boundAndPeriod, []types.Statistic{types.StatisticAverage})
	
	for _, datapoint := range metrics.Datapoints {
		log.Printf("metrics %v", datapoint.Average)
	}
	if err != nil {
		return err
	}

	if len(metrics.Datapoints) == 0 {
		return fmt.Errorf("\n getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, util.LogCloudWatchDimension(metricDimensions))
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)
	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, metricSampleCount, metricSampleCount, boundAndPeriod); !ok {
		return fmt.Errorf("\n metric %s is not within sample count bound [ %d, %d]", metricName, metricSampleCount, metricSampleCount)
	}

	// Validate if the corresponding metrics are within the acceptable range [acceptable value +- 10%]
	actualMetricValue := *metrics.Datapoints[0].Average
	upperBoundValue := metricValue * (1 + metricErrorBound)
	lowerBoundValue := metricValue * (1 - metricErrorBound)

	if metricValue != 0.0 && (actualMetricValue < lowerBoundValue || actualMetricValue > upperBoundValue) {
		return fmt.Errorf("\n metric %s value %f is different from the actual value %f", metricName, metricValue, actualMetricValue)
	}

	return nil
}

func getEMFSchemaAndValidateFunction(emfSchemaName string) (*jsonschema.Schema, func(string) bool) {
	switch emfSchemaName {
	case "emf_metrics_schema":
		// The validator will generate EMF metrics with InstanceId as a dimension
		// https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/internal/common/metrics.go#L183-L210
		// Therefore, validate if the InstanceId is there. There will be changes for ECS Fargate and EKS Fargate
		// but focus on EC2 first
		return jsonschema.Must(EmfSchema), func(s string) bool { return strings.Contains(s, "\"InstanceId\"") }
	default:
		log.Printf("Empty metric schema %s and will fail the validation",emfSchemaName)
		return nil, func(_ string) bool { return false }
	}
}
