// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models

import "time"

type ValidatorFactory interface {
	InitValidation() error
	StartValidation(startTime, endTime time.Time) error
	EndValidation() error
}
