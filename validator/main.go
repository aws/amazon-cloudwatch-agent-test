// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators"
)

var (
	configPath      = flag.String("validator-config", "", "A yaml depicts test information")
	preparationMode = flag.Bool("preparation-mode", false, "Prepare all the resources for the validation (e.g set up config) ")
)

func main() {
	flag.Parse()

	startTime := time.Now()
	vConfig, err := models.NewValidateConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to create validation config : %v \n", err)
	}

	if *preparationMode {
		if err = prepare(vConfig); err != nil {
			log.Fatalf("Prepare for validation failed: %v \n", err)
		}

		os.Exit(0)
	}
	err = validate(vConfig)
	if err != nil {
		log.Fatalf("Failed to validate: %v", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.Printf("Finish validation in %v minutes", duration.Minutes())

}

func validate(vConfig models.ValidateConfig) error {
	const maxRetry = 1
	var err error
	for i := 0; i < maxRetry; i++ {
		err = validators.LaunchValidator(vConfig)

		if err == nil {
			return nil
		}
		time.Sleep(30 * time.Second)
		log.Printf("test case: %s, validate type: %s, error: %v", vConfig.GetTestCase(), vConfig.GetValidateType(), err)
		continue
	}

	return fmt.Errorf("test case: %s, validate type: %s, error: %v", vConfig.GetTestCase(), vConfig.GetValidateType(), err)

}

func prepare(vConfig models.ValidateConfig) error {
	var (
		dataType            = vConfig.GetDataType()
		dataRate            = vConfig.GetDataRate()
		agentConfigFilePath = vConfig.GetCloudWatchAgentConfigPath()
	)
	switch dataType {
	case "logs":
		common.GenerateLogConfig(dataRate, agentConfigFilePath)
	default:
	}

	return nil
}
