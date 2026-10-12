// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package status

type TestStatus string

const (
	SUCCESSFUL TestStatus = "Successful"
	FAILED     TestStatus = "Failed"
	// SKIPPED marks a check that had nothing to observe. It does not fail the group.
	SKIPPED TestStatus = "Skipped"
)
