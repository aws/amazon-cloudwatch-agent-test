// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	"os"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func getRegion(env *environment.MetaData) string {
	if env.Region != "" {
		return env.Region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-west-2"
}
