// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package validators

import (
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
)

type performanceValidator struct {
	vConfig models.ValidateConfig
}

var _ ValidatorFactory = (*performanceValidator)(nil)

func (t *performanceValidator) initValidation() (err error) {
	var (
		datapointPeriod     = t.vConfig.GetDataPointPeriod()
		agentConfigFilePath = t.vConfig.GetCloudWatchAgentConfigPath()
		dataType            = t.vConfig.GetDataType()
		dataRate            = t.vConfig.GetDataRate()
		receivers, _, _     = t.vConfig.GetOtelConfig()
	)

	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, datapointPeriod, dataRate)
	default:
		err = common.StartSendingMetrics(receivers, datapointPeriod, dataRate)
	}

	return err
}

func (t *performanceValidator) startValidation() error {
	return nil
}

func (p *performanceValidator) buildDimension() []types.DimensionFilter {
	ec2InstanceId := awsservice.GetInstanceId()

	return []types.DimensionFilter{
		types.DimensionFilter{
			Name:  aws.String("InstanceId"),
			Value: aws.String(ec2InstanceId),
		},
		types.DimensionFilter{
			Name:  aws.String("exe"),
			Value: aws.String("cloudwatch-agent"),
		},
		types.DimensionFilter{
			Name:  aws.String("process_name"),
			Value: aws.String("amazon-cloudwatch-agent"),
		},
	}
}
