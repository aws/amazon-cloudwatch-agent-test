// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_sd

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/suite"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

/*
Purpose:
1) Validate ECS ServiceDiscovery via DockerLabels by publishing Prometheus EMF to CW  https://github.com/aws/amazon-cloudwatch-agent/blob/main/internal/ecsservicediscovery/README.md
2) Detect the changes in metadata endpoint for ECS Container Agent https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/util/ecsutil/ecsutil.go#L67-L75


Implementation:
1) Check if the LogGroupFormat correctly scrapes the clusterName from metadata endpoint (https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38)
2) Check if expected Prometheus EMF data is correctly published as logs and metrics to CloudWatch
*/

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &ECSServiceDiscoveryTestRunner{},
		},
	}
)

var _ test_runner.ITestRunner = (*ECSServiceDiscoveryTestRunner)(nil)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestECSServiceDiscoveryTestSuite(t *testing.T) {
	suite.Run(t, new(ECSServiceDiscoveryTestSuite))
}

type ECSServiceDiscoveryTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *ECSServiceDiscoveryTestSuite) GetSuiteName() string {
	return "ECSServiceDiscovery"
}

func (suite *ECSServiceDiscoveryTestSuite) TestAllInSuite() {
	
	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "ECS ServiceDiscovery Test Suite Failed")
}
