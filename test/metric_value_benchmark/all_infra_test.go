// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type AllInfraTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*AllInfraTestRunner)(nil)

func (r *AllInfraTestRunner) GetTestName() string {
	return "AllInfra"
}

func (r *AllInfraTestRunner) GetAgentConfigFileName() string {
	return "all_infra_config.json"
}

func (r *AllInfraTestRunner) GetAgentRunDuration() time.Duration {
	return 60 * time.Second
}

func (r *AllInfraTestRunner) GetMeasuredMetrics() []string {
	runners := r.infraRunners()
	var metrics []string
	for _, runner := range runners {
		metrics = append(metrics, runner.GetMeasuredMetrics()...)
	}
	return metrics
}

func (r *AllInfraTestRunner) SetupBeforeAgentRun() error {
	if getNetworkInterface() == "docker0" {
		// Best-effort: restart docker to ensure docker0 interface is up.
		// Ignored in environments without docker (e.g. localtest container).
		_ = common.RunCommands([]string{"sudo systemctl restart docker"})
	}
	return r.SetUpConfig()
}

func (r *AllInfraTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult
	for _, runner := range r.infraRunners() {
		group := runner.Validate()
		results = append(results, group.TestResults...)
	}
	return status.TestGroupResult{
		Name:        r.GetTestName(),
		TestResults: results,
	}
}

func (r *AllInfraTestRunner) infraRunners() []test_runner.ITestRunner {
	return []test_runner.ITestRunner{
		&CPUTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&MemTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&DiskTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&DiskIOTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&NetTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&NetStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&SwapTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&ProcessesTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
		&ProcStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: r.DimensionFactory}},
	}
}
