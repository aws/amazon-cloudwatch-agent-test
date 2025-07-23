// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package eks

import (
	"flag"
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
}

func TestEKSPerformance(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testMetricsMap, err := GetEKSPerformanceMetrics(env.PerformanceMetricMapName)
	if err != nil {
		t.Fatalf("Unable to get EKS performance metrics: %v", err)
	}

	var testResults []status.TestResult
	for _, metric := range testMetricsMap.Metrics {

		log.Printf("Fetching dimensions")
		dimensions := GetMetricDimensions(metric, env)

		log.Printf("Fetching metric from CloudWatch : %v", metric)
		testResults = append(testResults, ValidatePerformanceMetrics(metric.Name, metric.Threshold, dimensions))
	}

	res := status.TestGroupResult{
		Name:        env.PerformanceTestName,
		TestResults: testResults,
	}

	if res.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", res.Name)
		t.Fail()
	}
}
