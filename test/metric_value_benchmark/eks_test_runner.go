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

type IEKSTestRunner interface {
	validate(e *environment.MetaData) status.TestGroupResult
	getTestName() string
	getAgentConfigFileName() string
	getAgentRunDuration() time.Duration
	getMeasuredMetrics() []string
}

type EKSTestRunner struct {
	runner IEKSTestRunner
	env    environment.MetaData
}

func (t *EKSTestRunner) Run(s test_runner.ITestSuite, e *environment.MetaData) {
	name := t.runner.getTestName()
	log.Printf("Running %s", name)
	dur := t.runner.getAgentRunDuration()
	time.Sleep(dur)

	res := t.runner.validate(e)
	s.AddToSuiteResult(res)
	if res.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}
