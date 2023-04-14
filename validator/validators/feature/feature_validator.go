// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package feature

import (
	"time"

	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
)

type FeatureValidator struct {
	vConfig models.ValidateConfig
	models.ValidatorFactory
}

var _ models.ValidatorFactory = (*FeatureValidator)(nil)

func NewFeatureValidator(vConfig models.ValidateConfig) models.ValidatorFactory {
	return &FeatureValidator{
		vConfig:          vConfig,
		ValidatorFactory: basic.NewBasicValidator(vConfig),
	}
}

func (s *FeatureValidator) GenerateLoad() error {
	var (
		multiErr              error
		metricSendingInterval = time.Minute
		logGroup              = awsservice.GetInstanceId()
		metricNamespace       = s.vConfig.GetMetricNamespace()
		dataRate              = s.vConfig.GetDataRate()
		agentCollectionPeriod = s.vConfig.GetAgentCollectionPeriod()
		agentConfigFilePath   = s.vConfig.GetCloudWatchAgentConfigPath()
		receivers             = s.vConfig.GetPluginsConfig()
	)

	if err := common.StartLogWrite(agentConfigFilePath, agentCollectionPeriod, metricSendingInterval, dataRate); err != nil {
		multiErr = multierr.Append(multiErr, err)
	}

	// Sending metrics based on the receivers; however, for scraping plugin  (e.g prometheus), we would need to scrape it instead of sending
	for _, receiver := range receivers {
		if err := common.StartSendingMetrics(receiver, agentCollectionPeriod, metricSendingInterval, dataRate, logGroup, metricNamespace); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	return multiErr
}
