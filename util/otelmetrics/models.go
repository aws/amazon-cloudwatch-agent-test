// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"strings"
	"time"
)

// MetricScope represents the scope level at which a metric is produced.
type MetricScope int

const (
	ScopeNode MetricScope = iota
	ScopePod
	ScopeContainer
	ScopeCluster
)

func (s MetricScope) String() string {
	switch s {
	case ScopeNode:
		return "node"
	case ScopePod:
		return "pod"
	case ScopeContainer:
		return "container"
	case ScopeCluster:
		return "cluster"
	default:
		return fmt.Sprintf("MetricScope(%d)", int(s))
	}
}

// MetricLabels holds parsed OTLP labels grouped by ZIP-0006 scope.
// Always use NewMetricLabels() to avoid nil map panics.
type MetricLabels struct {
	Resource        map[string]string
	Instrumentation map[string]string
	Datapoint       map[string]string
	AWS             map[string]string
	AWSCloudWatch   map[string]string
}

// NewMetricLabels returns a MetricLabels with all maps initialized.
func NewMetricLabels() MetricLabels {
	return MetricLabels{
		Resource:        make(map[string]string),
		Instrumentation: make(map[string]string),
		Datapoint:       make(map[string]string),
		AWS:             make(map[string]string),
		AWSCloudWatch:   make(map[string]string),
	}
}

// ParseLabels categorizes raw PromQL label keys into ZIP-0006 scopes.
//
// Scope resolution uses longest-prefix matching:
//  1. @aws.cloudwatch.* → AWSCloudWatch
//  2. @aws.* → AWS
//  3. @resource.* → Resource
//  4. @instrumentation.* → Instrumentation
//  5. @datapoint.* → Datapoint (explicit)
//  6. Everything else → Datapoint (implicit default)
//
// The __name__ key is excluded from all scopes.
func ParseLabels(metric map[string]string) MetricLabels {
	labels := NewMetricLabels()
	for key, value := range metric {
		switch {
		case key == "__name__":
			continue
		case strings.HasPrefix(key, "@aws.cloudwatch."):
			labels.AWSCloudWatch[strings.TrimPrefix(key, "@aws.cloudwatch.")] = value
		case strings.HasPrefix(key, "@aws."):
			labels.AWS[strings.TrimPrefix(key, "@aws.")] = value
		case strings.HasPrefix(key, "@resource."):
			labels.Resource[strings.TrimPrefix(key, "@resource.")] = value
		case strings.HasPrefix(key, "@instrumentation."):
			labels.Instrumentation[strings.TrimPrefix(key, "@instrumentation.")] = value
		case strings.HasPrefix(key, "@datapoint."):
			labels.Datapoint[strings.TrimPrefix(key, "@datapoint.")] = value
		default:
			labels.Datapoint[key] = value
		}
	}
	return labels
}

// FormatPromQL formats labels back into a PromQL label selector string.
func (ml MetricLabels) FormatPromQL() string {
	var parts []string
	for key, value := range ml.Resource {
		parts = append(parts, `"@resource.`+key+`"="`+EscapePromQLValue(value)+`"`)
	}
	for key, value := range ml.Instrumentation {
		parts = append(parts, `"@instrumentation.`+key+`"="`+EscapePromQLValue(value)+`"`)
	}
	for key, value := range ml.AWSCloudWatch {
		parts = append(parts, `"@aws.cloudwatch.`+key+`"="`+EscapePromQLValue(value)+`"`)
	}
	for key, value := range ml.AWS {
		parts = append(parts, `"@aws.`+key+`"="`+EscapePromQLValue(value)+`"`)
	}
	for key, value := range ml.Datapoint {
		parts = append(parts, key+`="`+EscapePromQLValue(value)+`"`)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// AllLabels returns all labels as a flat map with full scope prefixes.
func (ml MetricLabels) AllLabels() map[string]string {
	result := make(map[string]string)
	for k, v := range ml.Resource {
		result["@resource."+k] = v
	}
	for k, v := range ml.Instrumentation {
		result["@instrumentation."+k] = v
	}
	for k, v := range ml.AWSCloudWatch {
		result["@aws.cloudwatch."+k] = v
	}
	for k, v := range ml.AWS {
		result["@aws."+k] = v
	}
	for k, v := range ml.Datapoint {
		result[k] = v
	}
	return result
}

// EscapePromQLValue escapes a string for use as a PromQL label value inside double quotes.
func EscapePromQLValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

// MetricResult represents a single metric query result from the OTLP PromQL API.
type MetricResult struct {
	MetricName  string
	Labels      MetricLabels
	Value       float64
	Timestamp   time.Time
	IsHistogram bool
}

// MetricDefinition describes everything testable about a single metric.
type MetricDefinition struct {
	Name           string
	MetricType     string // "counter", "gauge", "histogram", "summary"
	Scope          MetricScope
	ExpectedLabels []string
	Unit           string // "" if unset
}
