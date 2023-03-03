// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
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

func (s *PerformanceValidator) GenerateLoad() (err error) {
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

func (s *PerformanceValidator) CheckData(startTime, endTime time.Time) error {
	return nil
}

func (s *PerformanceValidator) Cleanup() error {
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
