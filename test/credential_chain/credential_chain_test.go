// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package credential_chain

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestCredentialChainTestSuite(t *testing.T) {
	suite.Run(t, new(CredentialChainTestSuite))
}
