// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models

import "time"

// ValidatorFactory will be an interface for every validator and signals the validation process
type ValidatorFactory interface {
	InitValidation() error
	StartValidation(startTime, endTime time.Time) error
	EndValidation() error
}
