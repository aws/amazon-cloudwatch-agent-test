// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/nvidia_gpu"
	"github.com/aws/amazon-cloudwatch-agent-test/test/restart"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
	"github.com/aws/amazon-cloudwatch-agent-test/validator/validators"
)

var (
	configPath      = flag.String("validator-config", "", "A yaml depicts test information")
	preparationMode = flag.Bool("preparation-mode", false, "Prepare all the resources for the validation (e.g set up config) ")
	testName        = flag.String("test-name", "", "Test name to execute")
	assumeRoleArn   = flag.String("role-arn", "", "Arn for assume IAM role if any")
)

func main() {
	flag.Parse()

	startTime := time.Now()

	// validator calls test code to get around OOM issue on windows hosts while running go test
	if len(*configPath) == 0 && len(*testName) > 0 {
		// execute test without parsing or processing configuration yaml

		var err error

		splitNames := strings.Split(*testName, "/")
		switch splitNames[len(splitNames)-1] {
		case "restart":
			err = restart.Validate()
		case "nvidia_gpu":
			err = nvidia_gpu.Validate()
		}

		if err != nil {
			log.Fatalf("Validator failed with %s: %v", *testName, err)
		}
	} else {
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
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.Printf("Finish validation in %v minutes", duration.Minutes())

}

func validate(vConfig models.ValidateConfig) error {
	var err error
	for i := 0; i < awsservice.StandardRetries; i++ {
		err = validators.LaunchValidator(vConfig)

		if err == nil {
			log.Printf("Test case: %s, validate type: %s has been successfully validated", vConfig.GetTestCase(), vConfig.GetValidateType())
			return nil
		}
		time.Sleep(60 * time.Second)
		log.Printf("test case: %s, validate type: %s, error: %v", vConfig.GetTestCase(), vConfig.GetValidateType(), err)
		continue
	}

	return fmt.Errorf("test case: %s, validate type: %s, error: %v", vConfig.GetTestCase(), vConfig.GetValidateType(), err)

}

func prepare(vConfig models.ValidateConfig) error {
	var (
		err                 error
		dataType            = vConfig.GetDataType()
		numberLogsMonitored = vConfig.GetNumberMonitoredLogs()
		agentConfigFilePath = vConfig.GetCloudWatchAgentConfigPath()
	)
	switch dataType {
	case "logs":
		err = common.GenerateLogConfig(numberLogsMonitored, agentConfigFilePath)
	default:
	}

	return err
}
