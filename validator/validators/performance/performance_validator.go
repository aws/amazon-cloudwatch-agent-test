// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cenkalti/backoff/v4"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	ServiceName      = "AmazonCloudWatchAgent"
	DynamoDBDataBase = "CWAPerformanceMetrics"
)

var (
	metricsConvertToMB = []string{"memory_rss", "memory_swap", "memory_vms", "write_bytes", "bytes_sent"}
)

type PerformanceValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*PerformanceValidator)(nil)

func NewPerformanceValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &PerformanceValidator{
		vConfig: vConfig,
	}
}

func (s *PerformanceValidator) InitValidation() (err error) {
	var (
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		dataType              = s.vConfig.GetDataType()
		dataRate              = s.vConfig.GetDataRate()
		receivers, _, _       = s.vConfig.GetPluginsConfig()
	)
	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, dataRate)
	default:
		err = common.StartSendingMetrics(receivers, agentCollectionPeriod, dataRate)
	}

	return err
}

func (s *PerformanceValidator) StartValidation(startTime, endTime time.Time) error {
	metrics, err := s.GetPerformanceMetrics(startTime, endTime)
	if err != nil {
		return err
	}

	perfInfo := s.CalculateMetricStatsAndPackMetrics(metrics.MetricDataResults)

	err = s.SendPacketToDatabase(perfInfo)
	if err != nil {
		return err
	}
	return nil
}

func (s *PerformanceValidator) EndValidation() error {
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

func (s *PerformanceValidator) SendPacketToDatabase(perfInfo PerformanceInformation) error {
	var (
		receivers, processors, exporters = s.vConfig.GetPluginsConfig()
		commitHash, commitDate           = s.vConfig.GetCommitInformation()
		agentCollectionPeriod            = fmt.Sprint(s.vConfig.GetAgentCollectionPeriod().Seconds())
	)

	err := backoff.Retry(func() error {
		existingPerfInfo, err := awsservice.GetPacketInDatabase(DynamoDBDataBase, ServiceName, "CommitDate", fmt.Sprint(commitDate), perfInfo)
		if err != nil {
			return err
		}

		// Get the latest performance information from the database and update by merging the existing one
		// and finally replace the packet in the database
		maps.Copy(existingPerfInfo["Results"].(map[string]interface{}), perfInfo["Results"].(map[string]interface{}))
		finalPerfInfo := PackIntoPerformanceInformation(receivers, processors, exporters, agentCollectionPeriod, commitHash, commitDate, existingPerfInfo["Results"])

		err = awsservice.ReplacePacketInDatabase(DynamoDBDataBase, finalPerfInfo)

		if err != nil {
			return err
		}
		return nil
	}, awsservice.StandardExponentialBackoff)

	return err
}
func (s *PerformanceValidator) CalculateMetricStatsAndPackMetrics(metrics []types.MetricDataResult) PerformanceInformation {
	var (
		receivers, processors, exporters = s.vConfig.GetPluginsConfig()
		commitHash, commitDate           = s.vConfig.GetCommitInformation()
		dataRate                         = fmt.Sprint(s.vConfig.GetDataRate())
		agentCollectionPeriod            = s.vConfig.GetAgentCollectionPeriod().Seconds()
	)
	performanceMetricResults := make(map[string]Stats)

	for _, metric := range metrics {
		metricLabel := strings.Split(*metric.Label, " ")
		metricName := metricLabel[len(metricLabel)-1]
		metricValues := metric.Values
		//Convert every bytes to MB
		if slices.Contains(metricsConvertToMB, metricName) {
			for i, val := range metricValues {
				metricValues[i] = val / (1000000)
			}
		}
		log.Printf("Start calculate metric statictics for metric %s", metricName)
		metricStats := CalculateMetricStatisticsBasedOnDataAndPeriod(metricValues, agentCollectionPeriod)
		log.Printf("Finished calculate metric statictics for metric %s: %v", metricName, metricStats)
		performanceMetricResults[metricName] = metricStats
	}

	return PackIntoPerformanceInformation(receivers, processors, exporters, fmt.Sprint(agentCollectionPeriod), commitHash, commitDate, map[string]interface{}{dataRate: performanceMetricResults})
}

func (s *PerformanceValidator) GetPerformanceMetrics(startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {
	var (
		metricNamespace              = s.vConfig.GetMetricNamespace()
		validationMetric             = s.vConfig.GetMetricValidation()
		ec2InstanceId                = awsservice.GetInstanceId()
		performanceMetricDataQueries = []types.MetricDataQuery{}
	)
	log.Printf("Start getting performance metrics from CloudWatch")
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
		performanceMetricDataQueries = append(performanceMetricDataQueries, s.buildStressMetricQueries(metric.MetricName, metricNamespace, metricDimensions))
	}

	metrics, err := awsservice.GetMetricData(performanceMetricDataQueries, startTime, endTime)

	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (s *PerformanceValidator) buildStressMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) types.MetricDataQuery {
	metricInformation := types.Metric{
		Namespace:  aws.String(metricNamespace),
		MetricName: aws.String(metricName),
		Dimensions: metricDimensions,
	}

	metricDataQuery := types.MetricDataQuery{
		MetricStat: &types.MetricStat{
			Metric: &metricInformation,
			Period: aws.Int32(10),
			Stat:   aws.String(string(models.AVERAGE)),
		},
		Id: aws.String(strings.ToLower(metricName)),
	}
	return metricDataQuery
}

func PackIntoPerformanceInformation(receivers, processors, exporters []string, collectionPeriod, commitHash string, commitDate int64, result interface{}) PerformanceInformation {
	instanceAMI := awsservice.GetImageId()
	instanceType := awsservice.GetInstanceType()

	return PerformanceInformation{
		"Service":          ServiceName,
		"CommitDate":       commitDate,
		"CommitHash":       commitHash,
		"Receivers":        receivers,
		"Processors":       processors,
		"Exporters":        exporters,
		"Results":          result,
		"CollectionPeriod": collectionPeriod,
		"InstanceAMI":      instanceAMI,
		"InstanceType":     instanceType,
	}
}
