// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package credentials_file

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func TestAssumeRole(t *testing.T) {
	runner := test_runner.TestRunner{TestRunner: &CredentialsFileTestRunner{test_runner.BaseTestRunner{}}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("Credentials File Test failed")
		result.Print()
	}
}
