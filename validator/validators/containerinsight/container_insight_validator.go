// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package containerinsight

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/basic"
)

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
	return nil
}

func (s *ContainerInsightValidator) Cleanup() error {
	return nil
}
