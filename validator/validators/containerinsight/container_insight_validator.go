// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package feature

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"go.uber.org/multierr"
	"golang.org/x/exp/slices"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
)

const metricErrorBound = 0.1

type ContainerInsightValidator struct {
	vConfig models.ValidateConfig
	models.ValidatorFactory
}

var _ models.ValidatorFactory = (*ContainerInsightValidator)(nil)

func NewContainerInsightValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &ContainerInsightValidator{
		vConfig:          vConfig,
		ValidatorFactory: basic.NewBasicValidator(vConfig),
	}
}

func (s *ContainerInsightValidator) CheckData(startTime, endTime time.Time) error {
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
		err := s.ValidateMetric(metric.MetricName, metricNamespace, metricDimensions, metric.MetricValue, metric.MetricSampleCount, startTime, endTime)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}

func (s *ContainerInsightValidator) Cleanup() error {
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

func (s *ContainerInsightValidator) RollUpMetricValidation() ([]models.MetricValidation, error) {
	var (
		clusterName      string
		receiver         = s.vConfig.GetPluginsConfig()[0]
		validationMetric = s.vConfig.GetMetricValidation()
	)

	for _, dimension := range validationMetric[0].MetricDimension {
		if dimension.Name == "ClusterName" {
			clusterName = dimension.Value
		}
	}

	if receiver == "ecs_container_insight" {
		containers, err := awsservice.GetContainerInstances(clusterName)

		if err != nil {
			return nil, err
		}

		for index, metric := range validationMetric {
			for _, dimension := range metric.MetricDimension {
				if dimension.Name == "ContainerInstanceId" {
					slices.Delete(validationMetric, index, index)
					for _, container := range containers {
						validationMetric = append(validationMetric, models.MetricValidation{
							MetricName: metric.MetricName,
							MetricDimension: []models.MetricDimension{
								{
									Name:  "ClusterName",
									Value: clusterName,
								},
								{
									Name:  "InstanceId",
									Value: container.EC2InstanceId,
								},
								{
									Name:  "ContainerInstanceId",
									Value: container.ContainerInstanceId,
								},
							},
							MetricValue:       metric.MetricValue,
							MetricSampleCount: metric.MetricSampleCount,
						})
					}
				}
			}
		}
	} else if receiver == "eks_container_insight" {
		return nil, errors.New("eks_container_insight has not been supported yet.")
	}
	return validationMetric, nil
}

func (s *ContainerInsightValidator) ValidateMetric(metricName, metricNamespace string, metricDimensions []types.Dimension, metricValue float64, metricSampleCount int, startTime, endTime time.Time) error {
	var (
		boundAndPeriod = s.vConfig.GetAgentCollectionPeriod().Seconds()
	)

	metricQueries := s.buildMetricQueries(metricName, metricNamespace, metricDimensions)

	log.Printf("Start to collect and validate metric %s with the namespace %s, start time %v and end time %v", metricName, metricNamespace, startTime, endTime)

	metrics, err := awsservice.GetMetricData(metricQueries, startTime, endTime)
	if err != nil {
		return err
	}

	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return fmt.Errorf("getting metric %s failed with the namespace %s and dimension %v", metricName, metricNamespace, metricDimensions)
	}

	// Validate if the metrics are not dropping any metrics and able to backfill within the same minute (e.g if the memory_rss metric is having collection_interval 1
	// , it will need to have 60 sample counts - 1 datapoint / second)
	if ok := awsservice.ValidateSampleCount(metricName, metricNamespace, metricDimensions, startTime, endTime, metricSampleCount, metricSampleCount, int32(boundAndPeriod)); !ok {
		return fmt.Errorf("metric %s is not within sample count bound [ %f, %f]", metricName, boundAndPeriod, boundAndPeriod)
	}

	// Validate if the corresponding metrics are within the acceptable range [acceptable value +- 10%]
	actualMetricValue := metrics.MetricDataResults[0].Values[0]
	upperBoundValue := metricValue * (1 + metricErrorBound)
	lowerBoundValue := metricValue * (1 - metricErrorBound)

	if metricValue != 0.0 && (actualMetricValue < lowerBoundValue || actualMetricValue > upperBoundValue) {
		return fmt.Errorf("metric %s value %f is different from the actual value %f", metricName, metricValue, metrics.MetricDataResults[0].Values[0])
	}

	return nil
}

func (s *ContainerInsightValidator) buildMetricQueries(metricName, metricNamespace string, metricDimensions []types.Dimension) []types.MetricDataQuery {
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
