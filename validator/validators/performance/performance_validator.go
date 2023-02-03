// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"fmt"
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
	DynamoDBDataBase = "CWAPerformanceMetrics"
	RELEASE_NAME_ENV = "RELEASE_NAME"
	IS_RELEASE       = "isRelease"
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
		datapointPeriod     = s.vConfig.GetDataPointPeriod()
		agentConfigFilePath = s.vConfig.GetCloudWatchAgentConfigPath()
		dataType            = s.vConfig.GetDataType()
		dataRate            = s.vConfig.GetDataRate()
		receivers, _, _     = s.vConfig.GetOtelConfig()
	)
	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, datapointPeriod, dataRate)
	default:
		err = common.StartSendingMetrics(receivers, datapointPeriod, dataRate)
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
		receivers, processors, exporters = s.vConfig.GetOtelConfig()
		commitHash, commitDate           = s.vConfig.GetCommitInformation()
	)

	err := backoff.Retry(func() error {
		existingPerfInfo, err := awsservice.GetPacketInDatabase(DynamoDBDataBase, "CommitHash", commitHash, perfInfo)
		if err != nil {
			return err
		}

		// Get the latest performance information from the database and update by merging the existing one
		// and finally replace the packet in the database
		maps.Copy(existingPerfInfo["Results"].(map[string]interface{}), perfInfo["Results"].(map[string]interface{}))
		finalPerfInfo := PackIntoPerformanceInformation(receivers, processors, exporters, commitHash, commitDate, false, existingPerfInfo["Results"])

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
		receivers, processors, exporters = s.vConfig.GetOtelConfig()
		commitHash, commitDate           = s.vConfig.GetCommitInformation()
		dataRate                         = fmt.Sprint(s.vConfig.GetDataRate())
		datapointPeriod                  = s.vConfig.GetDataPointPeriod().Seconds()
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
		metricStats := CalculateMetricStatisticsBasedOnDataAndPeriod(metricValues, datapointPeriod)
		performanceMetricResults[metricName] = metricStats
	}

	return PackIntoPerformanceInformation(receivers, processors, exporters, commitHash, commitDate, false, map[string]interface{}{dataRate: performanceMetricResults})
}

func (s *PerformanceValidator) GetPerformanceMetrics(startTime, endTime time.Time) (*cloudwatch.GetMetricDataOutput, error) {
	var (
		metricNamespace              = s.vConfig.GetMetricNamespace()
		validationMetric             = s.vConfig.GetMetricValidation()
		ec2InstanceId                = awsservice.GetInstanceId()
		performanceMetricDataQueries = []types.MetricDataQuery{}
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
		performanceMetricDataQueries = append(performanceMetricDataQueries, s.buildStressMetricQueries(metric.MetricName, metricNamespace, metricDimensions))
	}

	// We are only interesting in the maxium metric values within the time range since the metrics sending are not distributed evenly; therefore,
	// only the maximum shows the correct usage reflection of CloudWatchAgent during that time
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

func PackIntoPerformanceInformation(receivers, processors, exporters []string, commitHash string, commitDate int64, isRelease bool, result interface{}) PerformanceInformation {
	return PerformanceInformation{
		"Receivers":  receivers,
		"Processors": processors,
		"Exporters":  exporters,
		"CommitHash": commitHash,
		"CommitDate": commitDate,
		"IsRelease":  isRelease,
		"Results":    result,
	}
}
