// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package sanity

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/agent"
)

func SanityCheck(t *testing.T) {
	err := agent.RunShellScript("resources/verifyWindowsCtlScript.ps1")
	if err != nil {
		t.Fatalf("Running sanity check failed")
	}
}
