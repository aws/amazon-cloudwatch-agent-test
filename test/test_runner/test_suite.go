// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type ITestSuite interface {
	SetupSuite()
	TearDownSuite()
	AddToSuiteResult(r status.TestGroupResult)
	TestSuiteName() string
}

type TestSuite struct {
	Result status.TestSuiteResult
}

var _ ITestSuite = (*TestSuite)(nil)

func (suite *TestSuite) SetupSuite() {
	fmt.Printf(">>>> Starting %s TestSuite", suite.TestSuiteName())
}

func (suite *TestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Printf(">>>> Finished %s TestSuite", suite.TestSuiteName())
}

func (suite *TestSuite) TestSuiteName() string {
	return "Base"
}

func (suite *TestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}
