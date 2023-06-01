// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package restart

func Validate() error {
	return LogCheck("resources/verifyRestartScript.ps1")
}
