// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package run_as_user

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// Let the agent run for 15 seconds. This will give agent enough time to change user
	agentRuntime     = 15 * time.Second
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	pidFile          = "/opt/aws/amazon-cloudwatch-agent/var/amazon-cloudwatch-agent.pid"
	root             = "root"
	cwagent          = "cwagent"
)

type input struct {
	user      string
	dataInput string
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestRunAsUser(t *testing.T) {

	parameters := []input{
		{dataInput: "resources/default.json", user: root},
		{dataInput: "resources/root.json", user: root},
		{dataInput: "resources/cwagent.json", user: cwagent},
	}

	for _, parameter := range parameters {
		t.Run(fmt.Sprintf("resource file location %s user %s", parameter.dataInput, parameter.user), func(t *testing.T) {
			common.CopyFile(parameter.dataInput, configOutputPath)
			common.StartAgent(configOutputPath, true, false)
			time.Sleep(agentRuntime)
			log.Printf("Agent has been running for : %s", agentRuntime.String())
			// Must read the pid file while agent is running
			pidOutput, err := common.RunCommand(common.CatCommand + pidFile)
			if err != nil {
				t.Fatalf("Error: %v", err)
			}
			processOwnerCommand := common.AppOwnerCommand
			currentOS := runtime.GOOS
			switch currentOS {
			case "darwin":
				processOwnerCommand = "ps -j -p "
			}
			agentOwnerOutput, err := common.RunCommand(processOwnerCommand + pidOutput)
			if err != nil {
				t.Fatalf("Error: %v", err)
			}

			processOwner := outputContainsTarget(agentOwnerOutput, parameter.user)
			common.StopAgent()
			if processOwner != true {
				t.Fatalf("App owner is not %s", parameter.user)
			}
		})
	}
}

func outputContainsTarget(output string, target string) bool {
	log.Printf("PID file %s", output)
	contains := strings.Contains(output, target)
	log.Printf("PID file contains target string %t", contains)
	return contains
}
