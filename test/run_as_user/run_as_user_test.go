// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package run_as_user

import (
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/agent"
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

func TestRunAsUser(t *testing.T) {

	parameters := []input{
		{dataInput: "resources/default.json", user: root},
		{dataInput: "resources/root.json", user: root},
		{dataInput: "resources/cwagent.json", user: cwagent},
	}

	for _, parameter := range parameters {
		t.Run(fmt.Sprintf("resource file location %s user %s", parameter.dataInput, parameter.user), func(t *testing.T) {
			agent.CopyFile(parameter.dataInput, configOutputPath)
			agent.StartAgent(configOutputPath, true)
			time.Sleep(agentRuntime)
			log.Printf("Agent has been running for : %s", agentRuntime.String())
			// Must read the pid file while agent is running
			pidOutput := agent.RunCommand(agent.CatCommand + pidFile)
			agentOwnerOutput := agent.RunCommand(agent.AppOwnerCommand + pidOutput)
			processOwner := outputContainsTarget(agentOwnerOutput, parameter.user)
			agent.StopAgent()
			if processOwner != true {
				t.Errorf("App owner is not %s", parameter.user)
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
