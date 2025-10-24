// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package workload_discovery

import (
	"fmt"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows"
	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func Validate() error {
	util.WriteEmbeddedScript()
	util.InstallJava17()

	instanceType := awsservice.GetInstanceType()
	var errors []string

	if strings.HasPrefix(instanceType, "g4dn") {
		if err := windows.RunNVIDIATest(); err != nil {
			errors = append(errors, "NVIDIA test failed: "+err.Error())
		}
	} else {
		if err := util.VerifyWorkloadsEmpty(); err != nil {
			errors = append(errors, "Initial workloads not empty: "+err.Error())
		}

		if err := windows.RunJVMTest(); err != nil {
			errors = append(errors, "JVM test failed: "+err.Error())
		}

		if err := windows.RunTomcatTest(); err != nil {
			errors = append(errors, "Tomcat test failed: "+err.Error())
		}

		if err := windows.RunKafkaTest(); err != nil {
			errors = append(errors, "Kafka test failed: "+err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("workload discovery validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}
