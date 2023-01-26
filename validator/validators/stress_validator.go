// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package validators

import (
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"go.uber.org/multierr"
)

type stressValidator struct {
	validatorFactory
	vConfig models.ValidateConfig
}

var _ ValidatorFactory = (*stressValidator)(nil)

func (s *stressValidator) initValidation() (err error) {
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

func (s *stressValidator) startValidation() error {
	var multiErr error

	cwaMemoryUsage, err := s.getStressMetric("memory_rss", "", []types.Dimension{})

	if err != nil {
		return err
	}

	if cwaMemoryUsage > 0 {
		multiErr = multierr.Append(multiErr, err)
	}

	return nil
}

func (s *stressValidator) getStressMetric(metricName, metricNamespace string, metricDimensions []types.Dimension) (*cloudwatch.GetMetricDataOutput, error) {
	var (
		datapointPeriod = s.vConfig.GetDataPointPeriod()
		endTime         = time.Now()
		startTime       = endTime.Add(-datapointPeriod)
	)

	stressMetricQueries := s.buildStressMetricQueries(metricName, metricNamespace, metricDimensions)

	return awsservice.AWS.CwmAPI.GetMetricData(startTime, endTime, stressMetricQueries)
}

func (s *stressValidator) buildStressMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) []types.MetricDataQuery {
	var (
		ec2InstanceId     = awsservice.GetInstanceId()
		datapointPeriod   = s.vConfig.GetDataPointPeriod()
		metricQueryPeriod = int32(datapointPeriod.Seconds())
	)

	metricDimensions = append(metricDimensions,
		types.Dimension{
			Name:  aws.String("InstanceId"),
			Value: aws.String(ec2InstanceId),
		})

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
				Stat:   aws.String("Average"),
			},
			Id: aws.String(strings.ToLower(metricName)),
		},
	}
	return metricDataQueries
}
