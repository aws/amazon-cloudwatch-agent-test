// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type EKSTestRunner struct {
	Runner ITestRunner
	Env    environment.MetaData
}

func (t *EKSTestRunner) Run(s ITestSuite, e *environment.MetaData) {
	name := t.Runner.GetTestName()
	log.Printf("Running %s", name)
	dur := t.Runner.GetAgentRunDuration()
	time.Sleep(dur)

	res := t.Runner.Validate()
	s.AddToSuiteResult(res)
	if res.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}
