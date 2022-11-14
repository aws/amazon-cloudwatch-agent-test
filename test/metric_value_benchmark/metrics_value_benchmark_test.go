// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/compute_type"
	"log"
	"os"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/stretchr/testify/suite"
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
	{testRunner: &NetTestRunner{}},
}

// [DONE] make tests run
// [DONE] understand if you are ec2/fargate, daemon/replica/sidecar/ by passing flags
// [DONE pass arguments I want from main.tf
// TODO: Test runner -> has agentRunnerStrategy(). Shared testRunner construct for ec2 & ecs
// TODO: agentRunnerStrategy(ECS) -> has members like ssmparam, clusterarn etc. still the same interface right?
// TODO: agentRunnerStrategies accepts files
// TODO: maybe after this I can make a PR before coveredTestList cleanup. Make it simple & static for test list.
//TODO: test e2e
//TODO: coveredTestList needs to be cleaned up. See my handwritten notes for ideas. (Todo)
// Based on the above, make a factory.
// Do this only for ecs for now, and a separate PR for ec2 changes? nah..not possible
var ecsTestRunners = []*ECSTestRunner{
	{testRunner: &CPUTestRunner{}},
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(os.Args[0])
	if env.ComputeType == compute_type.ECS {
		log.Printf("Environment compute type is ECS")
		for _, ecsTestRunner := range ecsTestRunners {
			ecsTestRunner.Run(suite, env)
		}
	}

	for _, testRunner := range testRunners {
		testRunner.Run(suite)
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
