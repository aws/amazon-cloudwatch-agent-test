// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package basic

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
)

type BasicValidator struct {
	vConfig models.ValidateConfig
}

var _ models.ValidatorFactory = (*BasicValidator)(nil)

func NewBasicValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &BasicValidator{
		vConfig: vConfig,
	}
}

func (s *BasicValidator) GenerateLoad() (err error) {
	var (
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		dataType              = s.vConfig.GetDataType()
		dataRate              = s.vConfig.GetDataRate()
		receiver              = s.vConfig.GetPluginsConfig()
	)
	switch dataType {
	case "logs":
		err = common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, dataRate)
	default:
		// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
		err = common.StartSendingMetrics(receiver, agentCollectionPeriod, dataRate)
	}

	return err
}

func (s *BasicValidator) CheckData(startTime, endTime time.Time) error {
	return nil
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
