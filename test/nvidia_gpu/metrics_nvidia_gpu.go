// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package nvidia_gpu

func Validate() error {
	// this is a placeholder as validator looks for it
	return nil
}
