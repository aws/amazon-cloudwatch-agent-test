// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package sanity

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

func SanityCheck(t *testing.T) {
	err := common.RunShellScript("resources/verifyUnixCtlScript.sh")
	if err != nil {
		t.Fatalf("Running sanity check failed")
	}
}
