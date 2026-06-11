// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !linux

package journald

import "errors"

func Validate() error {
	return errors.New("test unsupported by OS")
}
