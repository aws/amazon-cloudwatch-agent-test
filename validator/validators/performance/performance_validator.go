// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

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

type PerformanceValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*PerformanceValidator)(nil)

func NewPerformanceValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &PerformanceValidator{
		vConfig: vConfig,
	}
}

func (p *PerformanceValidator) InitValidation() (err error) {
	var (
		datapointPeriod     = p.vConfig.GetDataPointPeriod()
		agentConfigFilePath = p.vConfig.GetCloudWatchAgentConfigPath()
		dataType            = p.vConfig.GetDataType()
		dataRate            = p.vConfig.GetDataRate()
		receivers, _, _     = p.vConfig.GetOtelConfig()
	)

	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, datapointPeriod, dataRate)
	default:
		err = common.StartSendingMetrics(receivers, datapointPeriod, dataRate)
	}

	return err
}

func (p *PerformanceValidator) StartValidation(startTime, endTime time.Time) error {
	data, err := p.GetPerformanceMetrics(startTime, endTime)

	//@TODO check if metrics are zero remove them and make sure there are non-zero metrics existing
	if err != nil || data == nil {
		return fmt.Errorf("failed to get performance metric: %v", err)
	}

	_, err = dynamoDB.SendItem(data, tps)
	if err != nil {
		return fmt.Errorf("failed to upload metric data  to table : %v", err)
	}
	return nil
}

func (p *PerformanceValidator) EndValidation() error {
	var (
		dataType      = s.vConfig.GetDataType()
		ec2InstanceId = awsservice.GetInstanceId()
	)
	switch dataType {
	case "logs":
		awsservice.DeleteLogGroup(ec2InstanceId)
	default:
	}

	return nil
}

func (p *PerformanceValidator) GetPerformanceMetrics(startTime, endTime time.Time) (map[string]interface{}, error) {
	var (
		dataRate               = fmt.Sprint(p.vConfig.GetDataRate())
		boundAndPeriod         = p.vConfig.GetDataPointPeriod().Seconds()
		ec2InstanceId          = awsservice.GetInstanceId()
		performanceDataQueries = []types.MetricDataQuery{}
		validationMetric       = p.vConfig.GetMetricValidation()
		metricNamespace        = p.vConfig.GetMetricNamespace()
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
		performanceDataQueries = append(performanceDataQueries, p.buildPerformanceMetricQuery(metric.MetricName, metricNamespace, metricDimensions))
	}

	metrics, err := awsservice.GetMetricData(performanceDataQueries, startTime, endTime)
	if err != nil {
		return nil, err
	}
	if len(metrics.MetricDataResults) == 0 || len(metrics.MetricDataResults[0].Values) == 0 {
		return nil, fmt.Errorf("getting metric  failed with the namespace %s", metricNamespace)
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

func (p *PerformanceValidator) buildPerformanceMetricQuery(metricName, metricNamespace string, metricDimensions []types.Dimension) types.MetricDataQuery {
	var (
		metricQueryPeriod = int32(p.vConfig.GetDataPointPeriod().Seconds())
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
			Stat:   aws.String(string(models.AVERAGE)),
		},
		Id: aws.String(strings.ToLower(metricName)),
	}

	return metricDataQuery
}
