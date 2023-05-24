// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package restart

import (
	"testing"
)

func RestartCheck(t *testing.T) {
	err := LogCheck("resources/verifyRestartScript.ps1")
	if err != "" {
		t.Fatalf(err)
	}
}
