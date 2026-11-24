// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMainPID(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{name: "running", out: "MainPID=1234\n", want: "1234"},
		{name: "no main process", out: "MainPID=0\n", want: ""},
		{name: "empty output", out: "", want: ""},
		{name: "other properties around it", out: "ActiveState=active\nMainPID=42\nSubState=running\n", want: "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, parseMainPID(tc.out))
		})
	}
}
