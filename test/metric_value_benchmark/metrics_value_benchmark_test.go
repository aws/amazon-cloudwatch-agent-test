// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"flag"
	"fmt"
	"log"
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

//TODO: coveredTestList needs to be cleaned up. See my handwritten notes for ideas.
// TODO: somewhere here needs to use the coveredTestList
var ecsTestRunners = []*ECSTestRunner{
	{testRunner: &CPUTestRunner{}},
}

var clusterArn = flag.String("clusterArn", "", "Used to restart ecs task to apply new agent config")
var cwagentConfigSsmParamName = flag.String("cwagentConfigSsmParamName", "", "Used to set new cwa config")
var serviceName = flag.String("cwagentECSServiceName", "", "Used to restart ecs task to apply new agent config")

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	if clusterArn != nil {
		log.Printf("cluster name isn't nil")
		for _, ecsTestRunner := range ecsTestRunners {
			ecsTestRunner.Run(suite, cwagentConfigSsmParamName, clusterArn, serviceName)
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
