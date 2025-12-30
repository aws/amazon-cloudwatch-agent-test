// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ssm_document

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestSSMDocument(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}
