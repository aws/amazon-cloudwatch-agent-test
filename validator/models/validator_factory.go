// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models

import "time"

// ValidatorFactory will be an interface for every validator and signals the validation process
type ValidatorFactory interface {
	GenerateLoad() error
	CheckData(startTime, endTime time.Time) error
	Cleanup() error
}
