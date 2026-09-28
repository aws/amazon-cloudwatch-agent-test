// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration && windows

package vm

var platformMetrics = []string{}

// spanMetrics are Linux-only (the Windows runs produce no spanmetrics), so none are validated here.
var spanMetrics = []string{}
