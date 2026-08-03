// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package efa_ec2

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestEfaEC2Metrics(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("efa ec2 validation failed: %s", err)
	}
}
