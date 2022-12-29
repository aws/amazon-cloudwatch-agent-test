// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
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

// TODO: test this runAgentStrategy and then if it works, refactor the ec2 ones with this too. -> no do this later?

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	ecsTestRunners []*ECSTestRunner
	ec2TestRunners []*TestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*ECSTestRunner {
	if ecsTestRunners == nil {
		factory := &metric.MetricFetcherFactory{Env: env}

		ecsTestRunners = []*ECSTestRunner{
			{testRunner: &ContainerInsightsTestRunner{ECSBaseTestRunner{MetricFetcherFactory: factory}},
				agentRunStrategy: &ECSAgentRunStrategy{}},
		}
	}
	return ecsTestRunners
}

func getEc2TestRunners(env *environment.MetaData) []*TestRunner {
	if ec2TestRunners == nil {
		factory := &metric.MetricFetcherFactory{Env: env}
		ec2TestRunners = []*TestRunner{
			{testRunner: &DiskTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &NetStatTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &PrometheusTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &StatsdTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &EMFTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &CollectDTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &SwapTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &CPUTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &MemTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &ProcStatTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &DiskIOTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &NetTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
			{testRunner: &ProcessesTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
		}
	}
	return ec2TestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	if env.ComputeType == computetype.ECS {
		log.Print("Environment compute type is ECS")
		for _, ecsTestRunner := range getEcsTestRunners(env) {
			ecsTestRunner.Run(suite, env)
		}
	} else {
		for _, testRunner := range getEc2TestRunners(env) {
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
