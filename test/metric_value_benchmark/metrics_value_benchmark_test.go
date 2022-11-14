// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/compute_type"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/stretchr/testify/suite"
	"log"
	"testing"
)

const namespace = "MetricValueBenchmarkTest"

type MetricBenchmarkTestSuite struct {
	suite.Suite
	result status.TestSuiteResult
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricBenchmarkTestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.result.Print()
	fmt.Println(">>>> Finished MetricBenchmarkTestSuite")
}

var testRunners = []*TestRunner{
	{testRunner: &CPUTestRunner{}},
	{testRunner: &MemTestRunner{}},
	{testRunner: &ProcStatTestRunner{}},
	{testRunner: &DiskIOTestRunner{}},
}

// [DONE] make tests run
// [DONE] understand if you are ec2/fargate, daemon/replica/sidecar/ by passing flags
// [DONE] pass arguments I want from main.tf
// [DONE] pass env arguments to ecs runner
// [DONE] Test runner -> has agentRunnerStrategy(). Shared testRunner construct for ec2 & ecs
// [DONE] try query-json with incomplete dimensions -> nope
// [DONE] if not work, then figure out a way to get containerInstanceId and instanceId from ecs.. because I don't think I have it
// ListContainerInstances -> container instance arn -> DescribeContainerInstances -> Ec2InstanceId.. containerInstanceId might be a substring of arn. we'll see.
// yep it is:         "arn:aws:ecs:us-east-1:aws_account_id:container-instance/container_instance_ID"
// [DONE] Create these api calls
// [DONE]: ok... then at query time or so, call these apis.
// [DONE]: say we have it, then what? I guess test Runners have to have var CommonDimensions -> for cpu, instanceId. var CommonDimensions -> for
// [DONE]: intanceId getter is different for ec2 vs ecs... so ... DimensionStrategy needed? get env -> if ec2..
// TODO: [NOW] Test validation.. create container insights test runner
// TODO: uhhh make these things not happen for ec2 suite.go:77: test panicked: Invalid compute type  default
// TODO: ec2 & ecs whole test
// TODO: test this runAgentStrategy and then if it works, refactor the ec2 ones with this too. -> no do this later?
// TODO: merge conflict resolution
// TODO: maybe after this I can make a PR before coveredTestList cleanup. Make it simple & static for test list.
//TODO: test e2e
//TODO: coveredTestList needs to be cleaned up. See my handwritten notes for ideas. (Todo)
// Based on the above, make a factory.
// Do this only for ecs for now, and a separate PR for ec2 changes? nah..not possible

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	if env.ComputeType == compute_type.ECS {
		log.Printf("Environment compute type is ECS")
		var ecsTestRunners = []*ECSTestRunner{
			{testRunner: &ContainerInsightsTestRunner{ECSBaseTestRunner{MetricFetcherFactory: &metric.MetricFetcherFactory{Env: env}}},
				agentRunStrategy: &ECSAgentRunStrategy{}},
		}
		for _, ecsTestRunner := range ecsTestRunners {
			ecsTestRunner.Run(suite, env)
		}
	} else {
		var testRunners = []*TestRunner{
			{testRunner: &CPUTestRunner{BaseTestRunner{MetricFetcherFactory: &metric.MetricFetcherFactory{Env: env}}}},
			{testRunner: &MemTestRunner{BaseTestRunner{MetricFetcherFactory: &metric.MetricFetcherFactory{Env: env}}}},
		}
		for _, testRunner := range testRunners {
			testRunner.Run(suite)
		}
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.result.GetStatus(), "Metric Benchmark Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.result.TestGroupResults = append(suite.result.TestGroupResults, r)
}

func TestMetricValueBenchmarkSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
}

func isAllValuesGreaterThanOrEqualToZero(metricName string, values []float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}
	for _, value := range values {
		if value < 0 {
			log.Printf("Values are not all greater than or equal to zero for %v", metricName)
			return false
		}
	}
	log.Printf("Values are all greater than or equal to zero for %v", metricName)
	return true
}
