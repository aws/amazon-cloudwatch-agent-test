// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestAssumeRoleTestSuite(t *testing.T) {
	suite.Run(t, new(AssumeRoleTestSuite))
}
