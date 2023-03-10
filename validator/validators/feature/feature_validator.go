// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package feature

import (
	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
)

type FeatureValidator struct {
	vConfig models.ValidateConfig
	basic.BasicValidator
}

var _ models.ValidatorFactory = (*FeatureValidator)(nil)

func NewFeatureValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &FeatureValidator{
		vConfig: vConfig,
	}
}

func (s *FeatureValidator) GenerateLoad() error {
	var (
		multiErr              error
		dataRate              = s.vConfig.GetDataRate()
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		receiver              = s.vConfig.GetPluginsConfig()
	)

	err := common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, dataRate)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
	}

	// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
	err = common.StartSendingMetrics(receiver, agentCollectionPeriod, dataRate)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
	}

	return multiErr
}
