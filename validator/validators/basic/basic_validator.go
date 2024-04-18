// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package basic

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	apMetrics "github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go/aws"
	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/util"
)

const metricErrorBound = 0.1

type BasicValidator struct {
	vConfig models.ValidateConfig
}

var RetryCount = 1
var _ models.ValidatorFactory = (*BasicValidator)(nil)

func NewBasicValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &BasicValidator{
		vConfig: vConfig,
	}
}

func (s *BasicValidator) GenerateLoad() error {
	var (
		metricSendingInterval = 10 * time.Second
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
	case "traces":
		return traces.StartTraceGeneration(receiver, agentConfigFilePath, agentCollectionPeriod, metricSendingInterval)
	default:
		// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
		return common.StartSendingMetrics(receiver, agentCollectionPeriod, metricSendingInterval, dataRate, logGroup, metricNamespace)
	}
}

func (s *BasicValidator) CheckData(startTime, endTime time.Time) error {
	var (
		multiErr         error
		ec2InstanceId    = awsservice.GetInstanceId()
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
		logValidations   = s.vConfig.GetLogValidation()
	)

	for _, metric := range validationMetric {
		metricDimensions := []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(ec2InstanceId),
			},
		}
		for _, dimension := range metric.MetricDimension {
			metricDimensions = append(metricDimensions, cwtypes.Dimension{
				Name:  aws.String(dimension.Name),
				Value: aws.String(dimension.Value),
			})
		}
		environmentValue := "Generic"
		operationValue := "operation"
		serviceValue := "service-name"

		// Creating the first group of dimensions
		appSignalDimensions := []cwtypes.Dimension{
			{
				Name:  aws.String("HostedIn.Environment"),
				Value: aws.String(environmentValue),
			},
			{
				Name:  aws.String("Operation"),
				Value: aws.String(operationValue),
			},
			{
				Name:  aws.String("Service"),
				Value: aws.String(serviceValue),
			},
		}
		//App Signals metric testing (This is because we want to use a different checking method (same that was done for linux test)
		if metric.MetricName == "Latency" || metric.MetricName == "Fault" || metric.MetricName == "Error" {
			fetcher := apMetrics.MetricValueFetcher{}
			values, err := fetcher.Fetch("AppSignals", metric.MetricName, appSignalDimensions, "sum", 60)
			if err != nil {
				fmt.Printf("error getting fetching app signals metrics %v", err)
			}
			if !apMetrics.IsAllValuesGreaterThanOrEqualToExpectedValue(metric.MetricName, values, 0) {
				fmt.Printf("error value not the epected values%v", err)
			}
		} else {
			err := s.ValidateMetric(metric.MetricName, metricNamespace, metricDimensions, metric.MetricValue, metric.MetricSampleCount, startTime, endTime)
			if err != nil {
				multiErr = multierr.Append(multiErr, err)
			}
		}
		lookbackDuration := time.Duration(-5) * time.Minute
		timeNow := time.Now()

		traceIds, err := awsservice.GetTraceIDs(timeNow.Add(lookbackDuration), timeNow, "")
		if err != nil {
			fmt.Printf("error getting trace ids: %v", err)
			multiErr = multierr.Append(multiErr, err)
		} else {
			fmt.Printf("Trace IDs: %v\n", traceIds)
			if len(traceIds) > 0 {
				fmt.Println("Trace IDs look good")
			}
		}

	}
	for _, logValidation := range logValidations {
		err := s.ValidateLogs(logValidation.LogStream, logValidation.LogValue, logValidation.LogLevel, logValidation.LogSource, logValidation.LogLines, startTime, endTime)
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

func (s *BasicValidator) ValidateLogs(logStream, logLine, logLevel, logSource string, expectedMinimumEventCount int, startTime, endTime time.Time) error {
	logGroup := awsservice.GetInstanceId()
	log.Printf("Start to validate that substring '%s' has at least %d log event(s) within log group %s, log stream %s, between %v and %v", logLine, expectedMinimumEventCount, logGroup, logStream, startTime, endTime)
	return awsservice.ValidateLogs(
		logGroup,
		logStream,
		&startTime,
		&endTime,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertNoDuplicateLogs(),
		func(events []cwltypes.OutputLogEvent) error {
			var actualEventCount int
			for _, event := range events {
				message := *event.Message
				switch logSource {
				case "WindowsEvents":
					if logLevel != "" && strings.Contains(message, logLine) && strings.Contains(message, logLevel) {
						actualEventCount += 1
					}
				default:
					if strings.Contains(message, logLine) {
						actualEventCount += 1
					}
				}
			}
			if actualEventCount < expectedMinimumEventCount {
				return fmt.Errorf("log event count for %q in %s/%s between %v and %v is %d which is less than the expected %d", logLine, logGroup, logStream, startTime, endTime, actualEventCount, expectedMinimumEventCount)
			}
			return nil
		},
	)
}

func (s *BasicValidator) ValidateMetric(metricName, metricNamespace string, metricDimensions []cwtypes.Dimension, metricValue float64, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
	)

	metricQueries := s.buildMetricQueries(metricName, metricNamespace, metricDimensions)

	log.Printf("Start to collect and validate metric %s with the namespace %s, start time %v and end time %v \n", metricName, metricNamespace, startTime, endTime)

	metrics, err := awsservice.GetMetricData(metricQueries, startTime, endTime)
	if err != nil {
		return err
	}
	fmt.Printf("For metric: %v This is the metric data result: %v and the result values: %v", metricName, metrics.MetricDataResults, metrics.MetricDataResults[0].Values)
	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("\n getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, util.LogCloudWatchDimension(metricDimensions))
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)

	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, metricSampleCount-2, metricSampleCount, int32(boundAndPeriod)); !ok {
		return fmt.Errorf("\n metric %s is not within sample count bound [ %d, %d]", metricName, metricSampleCount-2, metricSampleCount)
	} else {
		fmt.Println("Yayyyyyyy!!!!!!! sample count is good for :", metricName)
	}

	// Validate if the corresponding metrics are within the acceptable range [acceptable value +- 10%]
	actualMetricValue := metrics.MetricDataResults[0].Values[0]
	upperBoundValue := metricValue * (1 + metricErrorBound)
	lowerBoundValue := metricValue * (1 - metricErrorBound)

	if metricValue != 0.0 && (actualMetricValue < lowerBoundValue || actualMetricValue > upperBoundValue) {
		return fmt.Errorf("\n metric %s value %f is different from the actual value %f", metricName, metricValue, metrics.MetricDataResults[0].Values[0])
	}

	return nil
}

func (s *BasicValidator) buildMetricQueries(metricName, metricNamespace string, metricDimensions []cwtypes.Dimension) []cwtypes.MetricDataQuery {
	var metricQueryPeriod = int32(s.vConfig.GetAgentCollectionPeriod().Seconds())

	metricInformation := cwtypes.Metric{
		Namespace:  aws.String(metricNamespace),
		MetricName: aws.String(metricName),
		Dimensions: metricDimensions,
	}

	metricDataQueries := []cwtypes.MetricDataQuery{
		{
			MetricStat: &cwtypes.MetricStat{
				Metric: &metricInformation,
				Period: &metricQueryPeriod,
				Stat:   aws.String(string(models.AVERAGE)),
			},
			Id: aws.String(strings.ToLower(metricName)),
		},
	}

	fmt.Println("Maybe better form of metric query")
	jsonBytes, err := json.MarshalIndent(metricDataQueries, "", "    ")
	if err != nil {
		// handle error
		log.Fatal(err)
	}
	fmt.Println(string(jsonBytes))
	return metricDataQueries
}
