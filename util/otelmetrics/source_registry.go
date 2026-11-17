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
	SourceLISCSI
	SourceControlPlane
	SourceKubeStateMetrics
	SourceKSMNodeScoped
	SourceKarpenter
	SourceKEDA
	SourceKServe
	SourceKnative
)

// SourceMapping pairs a MetricSource with its metric definitions.
type SourceMapping struct {
	Source  MetricSource
	Metrics []MetricDefinition
}

// SourceHostMapping maps a MetricSource to the host types that produce it.
// Nil means one query with only the cluster filter, which returns series with or without host keys.
type SourceHostMapping struct {
	Source    MetricSource
	HostTypes []string // nil = cluster filter only
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

// HostTypesFor returns the host types to split the metric's query by; nil means one query with
// only the cluster filter. Returns nil for cluster-scoped sources whatever their host mapping,
// and allHostTypes for unknown metrics.
func (sr *SourceRegistry) HostTypesFor(metricName string) []string {
	source, ok := sr.metricToSource[metricName]
	if !ok {
		return sr.allHostTypes
	}
	if clusterScoped(source) {
		return nil
	}
	return sr.sourceToHosts[source]
}

// IsClusterScoped returns true if the metric comes from a cluster-scoped source. Such a metric is
// read with only the cluster filter, so a series that wrongly gained host keys is still returned.
func (sr *SourceRegistry) IsClusterScoped(metricName string) bool {
	source, ok := sr.metricToSource[metricName]
	return ok && clusterScoped(source)
}

func clusterScoped(source MetricSource) bool {
	return source == SourceControlPlane || source == SourceKubeStateMetrics
}
