// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func GetEcsTestRunners(env *environment.MetaData) []*ECSTestRunner {
	factory := dimension.GetDimensionFactory(*env)

	ecsTestRunners := []*ECSTestRunner{
		{
			testRunner:       &ContainerInsightsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}},
			agentRunStrategy: &ECSAgentRunStrategy{},
			env:              *env,
		},
	}
	return ecsTestRunners
}

func GetEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	factory := dimension.GetDimensionFactory(*env)
	ec2TestRunners := []*test_runner.TestRunner{
		{TestRunner: &DiskTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &NetStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &PrometheusTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &CPUTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &MemTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &ProcStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &DiskIOTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &NetTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		// TODO: The following plugins are not fully supported by CCWA, hence disabled temporarily. Restore back once supported.
		//{TestRunner: &StatsdTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		//{TestRunner: &EMFTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		//{TestRunner: &CollectDTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &SwapTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		{TestRunner: &ProcessesTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
	}
	return ec2TestRunners
}
