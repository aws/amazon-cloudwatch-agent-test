// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

// MetricSource identifies a metric's origin collector/exporter.
type MetricSource int

const (
	SourceNodeExporter MetricSource = iota
	SourceCadvisor
	SourceKubeletstats
	SourceDCGM
	SourceNeuron
	SourceEFA
	SourceEBSCSI
	SourceControlPlane
	SourceKubeStateMetrics
	SourceKSMNodeScoped
)

// SourceMapping pairs a MetricSource with its metric definitions.
type SourceMapping struct {
	Source  MetricSource
	Metrics []MetricDefinition
}

// SourceHostMapping maps a MetricSource to the host types that produce it.
// Nil means cluster-scoped (query with host.type="").
type SourceHostMapping struct {
	Source    MetricSource
	HostTypes []string // nil = cluster-scoped
}

// SourceRegistry maps metric names to their MetricSource and resolves
// which host types to query for each source.
type SourceRegistry struct {
	metricToSource map[string]MetricSource
	sourceToHosts  map[MetricSource][]string
	allHostTypes   []string
}

// NewSourceRegistry builds a registry from metric definitions and host mappings.
// allHostTypes is the full list of host types in the cluster (fallback for unknown metrics).
// hostMappings defines which host types produce each source's metrics.
func NewSourceRegistry(allHostTypes []string, hostMappings []SourceHostMapping, metricMappings ...SourceMapping) *SourceRegistry {
	sourceToHosts := make(map[MetricSource][]string)
	for _, hm := range hostMappings {
		sourceToHosts[hm.Source] = hm.HostTypes
	}

	metricToSource := make(map[string]MetricSource)
	for _, m := range metricMappings {
		for _, md := range m.Metrics {
			metricToSource[md.Name] = m.Source
		}
	}

	return &SourceRegistry{
		metricToSource: metricToSource,
		sourceToHosts:  sourceToHosts,
		allHostTypes:   allHostTypes,
	}
}

// HostTypesFor returns the host types that can produce the given metric.
// Returns nil for cluster-scoped sources. Returns allHostTypes for unknown metrics.
func (sr *SourceRegistry) HostTypesFor(metricName string) []string {
	source, ok := sr.metricToSource[metricName]
	if !ok {
		return sr.allHostTypes
	}
	return sr.sourceToHosts[source]
}

// IsClusterScoped returns true if the metric comes from a cluster-scoped source.
func (sr *SourceRegistry) IsClusterScoped(metricName string) bool {
	source, ok := sr.metricToSource[metricName]
	if !ok {
		return false
	}
	return source == SourceControlPlane || source == SourceKubeStateMetrics
}
