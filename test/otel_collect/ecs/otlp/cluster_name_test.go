// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import "testing"

func TestClusterNameFromArn(t *testing.T) {
	cases := []struct {
		name string
		arn  string
		want string
	}{
		{"full arn", "arn:aws:ecs:us-east-2:123456789012:cluster/my-cluster", "my-cluster"},
		{"no slash", "my-cluster", "my-cluster"},
		{"empty", "", ""},
		{"trailing slash", "arn:aws:ecs:us-east-2:123456789012:cluster/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clusterNameFromArn(tc.arn); got != tc.want {
				t.Fatalf("clusterNameFromArn(%q) = %q, want %q", tc.arn, got, tc.want)
			}
		})
	}
}
