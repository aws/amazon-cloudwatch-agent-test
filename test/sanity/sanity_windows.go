// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package sanity

import "testing"

// NOTE: The Windows sanity check is intentionally disabled.
//
// Running this via `go test ./test/sanity -p 1 -v` on Windows runners
// requires compiling the entire util/common import graph, which transitively
// pulls in aws-sdk-go-v2/service/ec2 (1700+ files) causing it to timing out the job.
//
// It has been replaced by invoking the PowerShell script directly from
// terraform/ec2/win/main.tf:
//	powershell.exe -ExecutionPolicy Bypass -File test\sanity\resources\verifyWindowsCtlScript.ps1
func SanityCheck(t *testing.T) {
	t.Skip("Windows sanity check runs via verifyWindowsCtlScript.ps1 directly from terraform; see note above")
}
