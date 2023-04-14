// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package validators

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/feature"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/performance"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators/stress"
)

func NewValidator(vConfig models.ValidateConfig) (validator models.ValidatorFactory, err error) {
	switch vConfig.GetValidateType() {
	case "performance":
		validator = performance.NewPerformanceValidator(vConfig)
	case "feature":
		validator = feature.NewFeatureValidator(vConfig)
	case "stress":
		validator = stress.NewStressValidator(vConfig)
	default:
		return nil, fmt.Errorf("unknown validation type %s provided by test case %s", vConfig.GetValidateType(), vConfig.GetTestCase())
	}

	return validator, nil
}

func LaunchValidator(vConfig models.ValidateConfig) error {
	var (
		agentCollectionPeriod    = vConfig.GetAgentCollectionPeriod()
		startTimeValidation      = time.Now().Truncate(time.Minute).Add(time.Minute)
		endTimeValidation        = startTimeValidation.Add(agentCollectionPeriod)
		durationBeforeNextMinute = time.Until(startTimeValidation)
	)

	validator, err := NewValidator(vConfig)
	if err != nil {
		return err
	}

	log.Printf("Start to sleep %f s for the metric to be available in the beginning of next minute ", durationBeforeNextMinute.Seconds())
	time.Sleep(durationBeforeNextMinute)

	log.Printf("Start to generate load in %f s for the agent to collect and send all the metrics to CloudWatch within the datapoint period ", agentCollectionPeriod.Seconds())
	err = validator.GenerateLoad()
	if err != nil {
		return err

	}

	time.Sleep(agentCollectionPeriod)
	log.Printf("Start to sleep 120s for CloudWatch to process all the metrics")
	time.Sleep(2 * time.Minute)

	err = validator.CheckData(startTimeValidation, endTimeValidation)
	if err != nil {
		return err
	}

	err = validator.Cleanup()
	if err != nil {
		return err
	}

	return nil

}
