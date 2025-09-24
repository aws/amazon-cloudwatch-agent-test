// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package dualstack_endpoint

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestDualstackEndpoints(t *testing.T) {
	err := ValidateDualstackEndpoints()
	if err != nil {
		t.Fatalf("dualstack endpoint validation failed: %s", err)
	}
}
