// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func resultWithResource(attrs map[string]string) MetricResult {
	labels := NewMetricLabels()
	for k, v := range attrs {
		labels.Resource[k] = v
	}
	return MetricResult{Labels: labels}
}

func TestCheckDeprecatedSemconvRemoved(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		results := []MetricResult{resultWithResource(map[string]string{
			"server.address": "10.0.0.1", "server.port": "9100", "url.scheme": "http",
		})}
		require.NoError(t, CheckDeprecatedSemconvRemoved("node_load1", results))
	})

	t.Run("each deprecated key is caught", func(t *testing.T) {
		for _, key := range DeprecatedSemconvResourceKeys {
			results := []MetricResult{resultWithResource(map[string]string{
				key: "x", "k8s.node.name": "ip-10-0-0-1",
			})}
			err := CheckDeprecatedSemconvRemoved("node_load1", results)
			require.Error(t, err, "key %s went undetected", key)
			require.Contains(t, err.Error(), key)
			require.Contains(t, err.Error(), "ip-10-0-0-1")
		}
	})

	t.Run("empty results pass", func(t *testing.T) {
		require.NoError(t, CheckDeprecatedSemconvRemoved("node_load1", nil))
	})

	t.Run("one offending series among many is caught", func(t *testing.T) {
		results := []MetricResult{
			resultWithResource(map[string]string{"url.scheme": "http"}),
			resultWithResource(map[string]string{"http.scheme": "http"}),
		}
		require.Error(t, CheckDeprecatedSemconvRemoved("node_load1", results))
	})
}

func TestCheckCurrentSemconvPresent(t *testing.T) {
	t.Run("present on one series is enough", func(t *testing.T) {
		results := []MetricResult{
			resultWithResource(map[string]string{}),
			resultWithResource(map[string]string{"url.scheme": "http"}),
		}
		require.NoError(t, CheckCurrentSemconvPresent("node_load1", results))
	})

	t.Run("absent everywhere fails", func(t *testing.T) {
		results := []MetricResult{resultWithResource(map[string]string{"http.scheme": "http"})}
		err := CheckCurrentSemconvPresent("node_load1", results)
		require.Error(t, err)
		require.Contains(t, err.Error(), "url.scheme")
	})

	t.Run("empty value counts as absent", func(t *testing.T) {
		results := []MetricResult{resultWithResource(map[string]string{"url.scheme": ""})}
		require.Error(t, CheckCurrentSemconvPresent("node_load1", results))
	})

	t.Run("empty results fail", func(t *testing.T) {
		require.Error(t, CheckCurrentSemconvPresent("node_load1", nil))
	})
}

func TestCheckRawLabelsNotAtResourceScope(t *testing.T) {
	rawKeys := []string{"pod", "namespace", "container"}

	t.Run("semconv names at resource scope are fine", func(t *testing.T) {
		results := []MetricResult{resultWithResource(map[string]string{
			"k8s.pod.name": "p", "k8s.namespace.name": "n", "k8s.container.name": "c",
		})}
		require.NoError(t, CheckRawLabelsNotAtResourceScope("container_cpu_usage_seconds_total", results, rawKeys))
	})

	t.Run("each raw key is caught", func(t *testing.T) {
		for _, key := range rawKeys {
			results := []MetricResult{resultWithResource(map[string]string{
				key: "x", "k8s.node.name": "ip-10-0-0-1",
			})}
			err := CheckRawLabelsNotAtResourceScope("container_cpu_usage_seconds_total", results, rawKeys)
			require.Error(t, err, "key %s went undetected", key)
			require.Contains(t, err.Error(), key)
		}
	})

	t.Run("empty value still counts as present", func(t *testing.T) {
		results := []MetricResult{resultWithResource(map[string]string{"pod": ""})}
		require.Error(t, CheckRawLabelsNotAtResourceScope("container_cpu_usage_seconds_total", results, rawKeys))
	})

	t.Run("duplicate problems are reported once", func(t *testing.T) {
		results := []MetricResult{
			resultWithResource(map[string]string{"pod": "a", "k8s.node.name": "ip-10-0-0-1"}),
			resultWithResource(map[string]string{"pod": "b", "k8s.node.name": "ip-10-0-0-1"}),
		}
		err := CheckRawLabelsNotAtResourceScope("container_cpu_usage_seconds_total", results, rawKeys)
		require.Error(t, err)
		require.Equal(t, 1, strings.Count(err.Error(), "@resource.pod"))
	})
}
