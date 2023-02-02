// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func GetEcsTestRunners(_ *environment.MetaData) []*ECSTestRunner {
	return []*ECSTestRunner{} // no windows ECS
}

func GetEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	factory := dimension.GetDimensionFactory(*env)
	ec2TestRunners := []*test_runner.TestRunner{
		{TestRunner: &CPUTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
	}

	return ec2TestRunners
}
