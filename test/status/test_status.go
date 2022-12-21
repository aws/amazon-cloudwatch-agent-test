// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux
// +build linux

package status

type TestStatus string

const (
	SUCCESSFUL TestStatus = "Successful"
	FAILED     TestStatus = "Failed"
)
