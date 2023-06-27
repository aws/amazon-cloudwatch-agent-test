// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package sanity

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func SanityCheck(t *testing.T) {
	_, err := common.RunShellScript("resources/verifyUnixCtlScript.sh")
	if err != nil {
		t.Fatalf("Running sanity check failed")
	}
}
