// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
	"time"
)

type EKSTestRunner struct {
	runner test_runner.ITestRunner
	env    environment.MetaData
}

func (t *EKSTestRunner) Run(s test_runner.ITestSuite, e *environment.MetaData) {
	name := t.runner.GetTestName()
	log.Printf("Running %s", name)
	dur := t.runner.GetAgentRunDuration()
	time.Sleep(dur)

	res := t.runner.Validate()
	s.AddToSuiteResult(res)
	if res.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}
