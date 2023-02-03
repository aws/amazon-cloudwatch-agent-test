// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package stress

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/uuid"
)

type MetricPluginBoundValue map[string]map[string]map[string]float64

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
	var (
		ec2InstanceId    = awsservice.GetInstanceId()
		metricNamespace  = s.vConfig.GetMetricNamespace()
		validationMetric = s.vConfig.GetMetricValidation()
	)

	packet, err := s.GetPerformanceMetrics(startTime, endTime)
	if err != nil || packet == nil {
		return err
	}

	_, err = dynamoDB.SendItem(data, tps)
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

func (s *PerformanceValidator) GetPerformanceMetrics(startTime, endTime time.Time) (map[string]interface{}, error) {
	var (
		ec2InstanceId                = awsservice.GetInstanceId()
		metricNamespace              = s.vConfig.GetMetricNamespace()
		validationMetric             = s.vConfig.GetMetricValidation()
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

	//craft packet to be sent to database
	packet := make(map[string]interface{})
	//add information about current release/commit
	packet[PARTITION_KEY] = time.Now().Year()
	packet[HASH] = os.Getenv(SHA_ENV) //fmt.Sprintf("%d", time.Now().UnixNano())
	packet[COMMIT_DATE], _ = strconv.Atoi(os.Getenv(SHA_DATE_ENV))
	packet[IS_RELEASE] = false
	//add test metadata
	packet[TEST_ID] = uuid.New().String()
	testSettings := fmt.Sprintf("%d-%d", logNum, tps)
	testMetricResults := make(map[string]Stats)

	//add actual test data with statistics
	for _, result := range metrics.MetricDataResults {
		//convert memory bytes to MB
		if *result.Label == "procstat_memory_rss" {
			for i, val := range result.Values {
				result.Values[i] = val / (1000000)
			}
		}
		stats := CalcStats(result.Values)
		testMetricResults[*result.Label] = stats
	}
	packet[RESULTS] = map[string]map[string]Stats{testSettings: testMetricResults}
	return packet, nil
}

func (s *PerformanceValidator) buildStressMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) types.MetricDataQuery {
	var (
		metricQueryPeriod = int32(s.vConfig.GetDataPointPeriod().Seconds())
	)

	metricInformation := types.Metric{
		Namespace:  aws.String(metricNamespace),
		MetricName: aws.String(metricName),
		Dimensions: metricDimensions,
	}

	metricDataQuery := types.MetricDataQuery{
		MetricStat: &types.MetricStat{
			Metric: &metricInformation,
			Period: &metricQueryPeriod,
			Stat:   aws.String(string(models.MAXIMUM)),
		},
		Id: aws.String(strings.ToLower(metricName)),
	}
	return metricDataQuery
}
