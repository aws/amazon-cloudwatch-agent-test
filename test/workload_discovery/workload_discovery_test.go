// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package workload_discovery

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestWorkloadDiscovery(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("workload discovery validation failed: %s", err)
	}
}
