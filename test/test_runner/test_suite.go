// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type ITestSuite interface {
	AddToSuiteResult(r status.TestGroupResult)
}
