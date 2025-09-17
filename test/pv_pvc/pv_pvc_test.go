// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package pv_pvc

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	pvPvcMetricIndicator = "persistent_volume_"

	persistentVolumeClaimCount         = "persistent_volume_claim_count"
	persistentVolumeClaimStatusBound   = "persistent_volume_claim_status_bound"
	persistentVolumeClaimStatusPending = "persistent_volume_claim_status_pending"
	persistentVolumeClaimStatusLost    = "persistent_volume_claim_status_lost"
	persistentVolumeCount              = "persistent_volume_count"
)

var expectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		persistentVolumeClaimCount, persistentVolumeClaimStatusBound,
		persistentVolumeClaimStatusPending, persistentVolumeClaimStatusLost,
		persistentVolumeCount,
	},
	"ClusterName-Namespace": {
		persistentVolumeClaimCount, persistentVolumeClaimStatusBound,
		persistentVolumeClaimStatusPending, persistentVolumeClaimStatusLost,
	},
	"ClusterName-Namespace-PersistentVolumeClaimName": {
		persistentVolumeClaimCount, persistentVolumeClaimStatusBound,
		persistentVolumeClaimStatusPending, persistentVolumeClaimStatusLost,
	},
}

type PvPvcTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *PvPvcTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting PV/PVC Container Insights TestSuite")
}

func (suite *PvPvcTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished PV/PVC Container Insights TestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var eksTestRunners []*test_runner.EKSTestRunner

func getEksTestRunners(env *environment.MetaData) []*test_runner.EKSTestRunner {
	if eksTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		eksTestRunners = []*test_runner.EKSTestRunner{
			{
				Runner: &PvPvcTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EKS_PV_PVC", env},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *PvPvcTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	case computetype.EKS:
		log.Println("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "PV/PVC Container Test Suite Failed")
}

func (suite *PvPvcTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestPvPvcSuite(t *testing.T) {
	suite.Run(t, new(PvPvcTestSuite))
}

type PvPvcTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*PvPvcTestRunner)(nil)

func (t *PvPvcTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, pvPvcMetricIndicator, expectedDimsToMetricsIntegTest)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *PvPvcTestRunner) GetTestName() string {
	return t.testName
}

func (t *PvPvcTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *PvPvcTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *PvPvcTestRunner) GetMeasuredMetrics() []string {
	return nil
}
