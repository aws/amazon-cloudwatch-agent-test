// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package restart

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestAgentStatusAfterRestart(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("restart validation failed: %s", err)
	}
}
