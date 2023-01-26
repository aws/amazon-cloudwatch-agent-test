// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators"
)

var (
	configPath = flag.String("config-path", "test", "Testing Config Path")
)

func main() {
	flag.Parse()

	startTime := time.Now()
	vConfig, err := models.NewValidateConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to create validation config : %v", err)
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
	const maxRetry = 5
	var err error

	for i := 0; i < maxRetry; i++ {
		err = validators.LaunchValidator(vConfig)
		if err == nil {
			return nil
		}
		time.Sleep(30 * time.Second)
		continue
	}

	return fmt.Errorf("test case: %s, validate type: %s, error: %v", vConfig.GetTestCase(), vConfig.GetValidateType(), err)

}
