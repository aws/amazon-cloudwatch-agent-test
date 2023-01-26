// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package validators

import (
	"fmt"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
)

func NewValidator(vConfig models.ValidateConfig) (validator ValidatorFactory, err error) {
	switch vConfig.GetValidateType() {
	case "performance":
		validator = &stressValidator{}
	case "stress":
		validator = &performanceValidator{}
	default:
		return nil, fmt.Errorf("unknown validation type %s provided by test case %s", vConfig.GetValidateType(), vConfig.GetTestCase())
	}

	return validator, nil
}

func LaunchValidator(vConfig models.ValidateConfig) error {
	var validator ValidatorFactory
	switch vConfig.GetValidateType() {
	case "performance":
		validator = &stressValidator{vConfig: vConfig}
	case "stress":
		validator = &performanceValidator{vConfig: vConfig}
	default:
		return fmt.Errorf("unknown validation type %s provided by test case %s", vConfig.GetValidateType(), vConfig.GetTestCase())
	}

	err := validator.initValidation()
	if err != nil {
		return fmt.Errorf("initialize validation with validation type %s and test case %s failed : %v", vConfig.GetValidateType(), vConfig.GetTestCase(), err)

	}

	err = validator.startValidation()
	if err != nil {
		return fmt.Errorf("start validation with validation type %s and test case %s failed : %v", vConfig.GetValidateType(), vConfig.GetTestCase(), err)
	}

	return nil

}

type ValidatorFactory interface {
	initValidation() error
	startValidation() error
}

type validatorFactory struct{}

func (t *validatorFactory) initValidation() error {
	return nil
}

func (t *validatorFactory) startValidation() error {
	return nil
}
