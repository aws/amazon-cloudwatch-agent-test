// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"os"
	"reflect"
	"testing"
)

func TestResolveHostTypes(t *testing.T) {
	defaults := []string{"g4dn.xlarge", "g4dn.12xlarge"}

	t.Run("unset", func(t *testing.T) {
		t.Setenv(HostTypesEnvVar, "")
		os.Unsetenv(HostTypesEnvVar)
		if got := ResolveHostTypes(defaults); !reflect.DeepEqual(got, defaults) {
			t.Fatalf("got %v, want %v", got, defaults)
		}
	})

	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", defaults},
		{"whitespace_only", "  \t ", defaults},
		{"separators_only", " , ,, ", defaults},
		{"single", "g5.xlarge", []string{"g5.xlarge"}},
		{"multiple", "g5.xlarge,trn1.32xlarge", []string{"g5.xlarge", "trn1.32xlarge"}},
		{"trims_whitespace", "  g5.xlarge , trn1.32xlarge\t", []string{"g5.xlarge", "trn1.32xlarge"}},
		{"drops_empty_entries", ",g5.xlarge,, ,trn1.32xlarge,", []string{"g5.xlarge", "trn1.32xlarge"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(HostTypesEnvVar, tc.raw)
			if got := ResolveHostTypes(defaults); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ResolveHostTypes with %q: got %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResolveLabelExcludes(t *testing.T) {
	for _, name := range []string{PodLabelExcludesEnvVar, NodeLabelExcludesEnvVar} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	if got := ResolveLabelExcludes(); len(got.Pod) != 0 || len(got.Node) != 0 {
		t.Fatalf("unset: got %+v, want no excludes", got)
	}

	t.Setenv(PodLabelExcludesEnvVar, " app.kubernetes.io/* ,, team")
	t.Setenv(NodeLabelExcludesEnvVar, "karpenter.sh/nodepool")
	want := LabelExcludes{Pod: []string{"app.kubernetes.io/*", "team"}, Node: []string{"karpenter.sh/nodepool"}}
	if got := ResolveLabelExcludes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNodeGroupsFor(t *testing.T) {
	descriptions := map[string]string{"t3.medium": "standard"}
	got := NodeGroupsFor([]string{"t3.medium", "g5.xlarge"}, descriptions)
	want := []NodeGroup{{"t3.medium", "standard"}, {"g5.xlarge", "override"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
