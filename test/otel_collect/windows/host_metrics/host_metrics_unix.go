// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package host_metrics

import "errors"

// Validate is only implemented on Windows; this stub lets the validator build on other platforms.
func Validate() error {
	return errors.New("otel_collect windows host_metrics validation is only supported on Windows")
}
