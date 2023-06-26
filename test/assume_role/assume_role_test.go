// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func TestAssumeRole(t *testing.T) {
	runner := test_runner.TestRunner{TestRunner: &RoleTestRunner{test_runner.BaseTestRunner{}}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("Assume Role Test failed")
		result.Print()
	}
}
