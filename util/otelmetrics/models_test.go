// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"strings"
	"testing"
)

func TestParseLabelsScopes(t *testing.T) {
	t.Run("resource_labels", func(t *testing.T) {
		metric := map[string]string{
			"__name__": "cpu_usage", "@resource.k8s.cluster.name": "test-cluster",
			"@resource.k8s.pod.name": "nginx-abc123", "@resource.k8s.namespace.name": "default",
		}
		labels := ParseLabels(metric)
		if labels.Resource["k8s.cluster.name"] != "test-cluster" {
			t.Fatalf("got %q", labels.Resource["k8s.cluster.name"])
		}
		if len(labels.Resource) != 3 {
			t.Fatalf("expected 3 resource labels, got %d", len(labels.Resource))
		}
	})
	t.Run("aws_cloudwatch_before_aws", func(t *testing.T) {
		metric := map[string]string{
			"__name__": "x", "@aws.cloudwatch.namespace": "AWS/EC2", "@aws.region": "us-east-1",
		}
		labels := ParseLabels(metric)
		if labels.AWSCloudWatch["namespace"] != "AWS/EC2" {
			t.Fatalf("got %q", labels.AWSCloudWatch["namespace"])
		}
		if _, ok := labels.AWS["cloudwatch.namespace"]; ok {
			t.Fatal("cloudwatch.namespace leaked into AWS map")
		}
	})
	t.Run("__name__excluded", func(t *testing.T) {
		labels := ParseLabels(map[string]string{"__name__": "cpu", "@resource.x": "y"})
		if _, ok := labels.Datapoint["__name__"]; ok {
			t.Fatal("__name__ should be excluded")
		}
	})
	t.Run("all_scopes", func(t *testing.T) {
		metric := map[string]string{
			"__name__": "m", "@resource.a": "1", "@instrumentation.b": "2",
			"@aws.c": "3", "@aws.cloudwatch.d": "4", "e": "5",
		}
		labels := ParseLabels(metric)
		if len(labels.Resource) != 1 || len(labels.Instrumentation) != 1 ||
			len(labels.AWS) != 1 || len(labels.AWSCloudWatch) != 1 || len(labels.Datapoint) != 1 {
			t.Fatalf("scope counts wrong: R=%d I=%d A=%d AC=%d D=%d",
				len(labels.Resource), len(labels.Instrumentation),
				len(labels.AWS), len(labels.AWSCloudWatch), len(labels.Datapoint))
		}
	})
}

func TestFormatPromQL(t *testing.T) {
	t.Run("scoped_quoted", func(t *testing.T) {
		labels := NewMetricLabels()
		labels.Resource["k8s.cluster.name"] = "test"
		result := labels.FormatPromQL()
		if !strings.Contains(result, `"@resource.k8s.cluster.name"="test"`) {
			t.Fatalf("got %q", result)
		}
	})
	t.Run("datapoint_unquoted", func(t *testing.T) {
		labels := NewMetricLabels()
		labels.Datapoint["job"] = "node"
		result := labels.FormatPromQL()
		if !strings.Contains(result, `job="node"`) {
			t.Fatalf("got %q", result)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got := NewMetricLabels().FormatPromQL(); got != "{}" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestAllLabels(t *testing.T) {
	labels := NewMetricLabels()
	labels.Resource["a"] = "1"
	labels.AWS["b"] = "2"
	labels.Datapoint["c"] = "3"
	flat := labels.AllLabels()
	if flat["@resource.a"] != "1" || flat["@aws.b"] != "2" || flat["c"] != "3" {
		t.Fatalf("got %v", flat)
	}
}

func TestEscaping(t *testing.T) {
	if got := EscapePromQLValue(`a"b\c`); got != `a\"b\\c` {
		t.Fatalf("got %q", got)
	}
}

func TestNewMetricLabels(t *testing.T) {
	labels := NewMetricLabels()
	if labels.Resource == nil || labels.Instrumentation == nil ||
		labels.Datapoint == nil || labels.AWS == nil || labels.AWSCloudWatch == nil {
		t.Fatal("nil map in NewMetricLabels")
	}
}

func TestRoundTrip(t *testing.T) {
	original := map[string]string{
		"__name__": "cpu", "@resource.cluster": "c1",
		"@instrumentation.@name": "x", "@aws.account": "123",
		"@aws.cloudwatch.ns": "CI", "job": "node",
	}
	promql := ParseLabels(original).FormatPromQL()
	for _, exp := range []string{
		`"@resource.cluster"="c1"`, `"@instrumentation.@name"="x"`,
		`"@aws.account"="123"`, `"@aws.cloudwatch.ns"="CI"`, `job="node"`,
	} {
		if !strings.Contains(promql, exp) {
			t.Fatalf("missing %q in %q", exp, promql)
		}
	}
}
