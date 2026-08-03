// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// TestAssertLogsNotEmpty locks the sentinel contract the ECS service-discovery retry logic
// depends on: an empty event list yields an error matchable via errors.Is against
// ErrNoLogEvents, including when wrapped, and a non-empty list yields nil.
func TestAssertLogsNotEmpty(t *testing.T) {
	validator := awsservice.AssertLogsNotEmpty()

	if err := validator(nil); !errors.Is(err, awsservice.ErrNoLogEvents) {
		t.Fatalf("empty events: expected errors.Is(err, ErrNoLogEvents), got %v", err)
	}
	if wrapped := fmt.Errorf("scenario x: %w", validator(nil)); !errors.Is(wrapped, awsservice.ErrNoLogEvents) {
		t.Fatalf("wrapped empty events: expected errors.Is to match ErrNoLogEvents, got %v", wrapped)
	}
	if err := validator([]types.OutputLogEvent{{}}); err != nil {
		t.Fatalf("non-empty events: expected nil, got %v", err)
	}
}

// TestIsResourceNotFoundException covers the other half of the retry contract: RNF detection
// must match a direct or wrapped *types.ResourceNotFoundException and nothing else.
func TestIsResourceNotFoundException(t *testing.T) {
	rnf := &types.ResourceNotFoundException{}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"direct", rnf, true},
		{"wrapped", fmt.Errorf("get logs: %w", rnf), true},
		{"unrelated", errors.New("throttled"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := awsservice.IsResourceNotFoundException(tc.err); got != tc.want {
				t.Fatalf("IsResourceNotFoundException(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
