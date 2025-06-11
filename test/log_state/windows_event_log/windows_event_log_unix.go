// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package windows_event_log

import "errors"

func Validate() error {
	return errors.New("test unsupported by OS")
}
