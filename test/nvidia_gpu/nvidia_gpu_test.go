// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package nvidia_gpu

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestNvidiaGpuMetrics(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("nvidia gpu validation failed: %s", err)
	}
}
