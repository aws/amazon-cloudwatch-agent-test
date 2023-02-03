// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package validators

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/performance"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/stress"
)

func NewValidator(vConfig models.ValidateConfig) (validator models.ValidatorFactory, err error) {
	switch vConfig.GetValidateType() {
	case "stress":
		validator = stress.NewStressValidator(vConfig)
	case "performance":
		validator = performance.NewPerformanceValidator(vConfig)
	default:
		return nil, fmt.Errorf("unknown validation type %s provided by test case %s", vConfig.GetValidateType(), vConfig.GetTestCase())
	}

	return validator, nil
}

func LaunchValidator(vConfig models.ValidateConfig) error {
	var (
		datapointPeriod          = vConfig.GetDataPointPeriod()
		startTimeValidation      = time.Now().Truncate(time.Minute).Add(time.Minute)
		endTimeValidation        = startTimeValidation.Add(datapointPeriod)
		durationBeforeNextMinute = time.Until(startTimeValidation)
	)

	validator, err := NewValidator(vConfig)
	if err != nil {
		return err
	}

	log.Printf("Start to sleep %f s for the metric to be available in the beginning of next minute ", durationBeforeNextMinute.Seconds())
	time.Sleep(durationBeforeNextMinute)

	err = validator.InitValidation()
	if err != nil {
		return err

	}

	log.Printf("Start to sleep %f s for the agent to collect and send all the metrics to CloudWatch within the datapoint period ", datapointPeriod.Seconds())
	time.Sleep(datapointPeriod)

	log.Printf("Start to sleep 60s for CloudWatch to process all the metrics")
	time.Sleep(60 * time.Second)

	err = validator.StartValidation(startTimeValidation, endTimeValidation)
	if err != nil {
		return err
	}

	err = validator.EndValidation()
	if err != nil {
		return err
	}

	return nil

}
