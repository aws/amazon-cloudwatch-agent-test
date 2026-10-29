// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"os"
	"strings"
)

// HostTypesEnvVar holds a comma-separated list of instance types that replaces
// a suite's default host types, so a suite can run against a cluster with other nodes.
const HostTypesEnvVar = "OTEL_HOST_TYPES"

// ResolveHostTypes returns the host types from HostTypesEnvVar, or defaults when
// it is unset or lists no non-blank entries.
func ResolveHostTypes(defaults []string) []string {
	return parseHostTypes(os.Getenv(HostTypesEnvVar), defaults)
}

// PodLabelExcludesEnvVar and NodeLabelExcludesEnvVar hold the comma-separated
// filters.metrics.podLabels.exclude and nodeLabels.exclude patterns the cluster
// was deployed with. The denylist test logs the entries they render to, and fails
// for any other entry that removes a never-denied attribute.
const (
	PodLabelExcludesEnvVar  = "OTEL_POD_LABEL_EXCLUDES"
	NodeLabelExcludesEnvVar = "OTEL_NODE_LABEL_EXCLUDES"
)

// ResolveLabelExcludes returns the patterns from PodLabelExcludesEnvVar and
// NodeLabelExcludesEnvVar. Unset means the cluster has no label excludes.
func ResolveLabelExcludes() LabelExcludes {
	return LabelExcludes{
		Pod:  splitList(os.Getenv(PodLabelExcludesEnvVar)),
		Node: splitList(os.Getenv(NodeLabelExcludesEnvVar)),
	}
}

func parseHostTypes(raw string, defaults []string) []string {
	if hostTypes := splitList(raw); len(hostTypes) > 0 {
		return hostTypes
	}
	return defaults
}

// splitList returns the non-blank entries of a comma-separated list, trimmed.
func splitList(raw string) []string {
	var entries []string
	for _, entry := range strings.Split(raw, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

// NodeGroup is one instance type a suite expects nodes of.
type NodeGroup struct {
	InstanceType string
	Description  string
}

// NodeGroupsFor returns one group per host type, so OTEL_HOST_TYPES also moves the per-group
// tests. Types missing from descriptions are described as "override".
func NodeGroupsFor(hostTypes []string, descriptions map[string]string) []NodeGroup {
	groups := make([]NodeGroup, 0, len(hostTypes))
	for _, hostType := range hostTypes {
		description, ok := descriptions[hostType]
		if !ok {
			description = "override"
		}
		groups = append(groups, NodeGroup{InstanceType: hostType, Description: description})
	}
	return groups
}
