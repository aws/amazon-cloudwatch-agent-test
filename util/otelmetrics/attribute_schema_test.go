// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeySchemaCheck(t *testing.T) {
	s := KeySchema{
		Required:     []string{"k8s.cluster.name", "server.port"},
		Optional:     []string{"k8s.node.uid"},
		OpenPrefixes: []string{"k8s.node.label."},
	}

	t.Run("exact set passes", func(t *testing.T) {
		require.True(t, s.Check(map[string]string{"k8s.cluster.name": "c", "server.port": "9100"}).OK())
	})

	t.Run("optional and open-prefix keys are allowed", func(t *testing.T) {
		require.True(t, s.Check(map[string]string{
			"k8s.cluster.name": "c", "server.port": "9100",
			"k8s.node.uid": "u", "k8s.node.label.kubernetes.io/os": "linux",
		}).OK())
	})

	t.Run("a required key deleted by mistake is missing", func(t *testing.T) {
		c := s.Check(map[string]string{"k8s.cluster.name": "c", "net.host.port": "9100"})
		require.Equal(t, []string{"server.port"}, c.Missing)
	})

	t.Run("an undeclared key is unexpected", func(t *testing.T) {
		c := s.Check(map[string]string{"k8s.cluster.name": "c", "server.port": "9100", "net.host.port": "9100"})
		require.Equal(t, []string{"net.host.port"}, c.Unexpected)
	})

	t.Run("a key under another prefix is not open", func(t *testing.T) {
		c := s.Check(map[string]string{"k8s.cluster.name": "c", "server.port": "9100", "k8s.pod.label.app": "x"})
		require.Equal(t, []string{"k8s.pod.label.app"}, c.Unexpected)
	})

	t.Run("scope name and version are not attributes", func(t *testing.T) {
		require.True(t, s.Check(map[string]string{
			"k8s.cluster.name": "c", "server.port": "9100", "@name": "scope", "@version": "1",
		}).OK())
	})
}

func TestKeySchemaImplies(t *testing.T) {
	s := KeySchema{
		Optional: []string{"k8s.deployment.name", "k8s.workload.name", "k8s.workload.type"},
		Implies: MergeImplies(
			map[string][]string{"k8s.deployment.name": {"k8s.workload.name", "k8s.workload.type"}},
			Together("k8s.workload.name", "k8s.workload.type"),
		),
	}
	require.True(t, s.Check(map[string]string{}).OK(), "a bare pod has none of them")
	require.True(t, s.Check(map[string]string{
		"k8s.deployment.name": "d", "k8s.workload.name": "d", "k8s.workload.type": "Deployment",
	}).OK())
	require.Equal(t, []string{"k8s.deployment.name without k8s.workload.name", "k8s.deployment.name without k8s.workload.type"},
		s.Check(map[string]string{"k8s.deployment.name": "d"}).Broken, "set_workload removed")
	require.Equal(t, []string{"k8s.workload.name without k8s.workload.type"},
		s.Check(map[string]string{"k8s.workload.name": "d"}).Broken)
}

func TestSchemaTally(t *testing.T) {
	schema := GroupSchema{
		Resource: KeySchema{
			Required: []string{"k8s.pod.name"},
			Expected: []string{"k8s.node.name"},
			Implies:  Together("host.id", "host.name"),
			Optional: []string{"host.id", "host.name"},
		},
		Instrumentation: KeySchema{Required: []string{"cloudwatch.source"}},
	}
	record := func(tally *SchemaTally, resource map[string]string) {
		l := MetricLabels{Resource: resource, Instrumentation: map[string]string{"cloudwatch.source": "s"}}
		r, i := tally.Check(l)
		tally.Record(l, r, i)
	}

	ok := NewSchemaTally(schema)
	record(ok, map[string]string{"k8s.pod.name": "p1"}) // unscheduled
	record(ok, map[string]string{"k8s.pod.name": "p2", "k8s.node.name": "n"})
	require.Empty(t, ok.Problems())

	bad := NewSchemaTally(schema)
	record(bad, map[string]string{"k8s.pod.name": "p1", "host.id": "i"})
	record(bad, map[string]string{"extra": "x"})
	require.Equal(t, []string{
		"@resource.host.id without @resource.host.name on 1 of 2 series",
		"missing required @resource.k8s.pod.name on 1 of 2 series",
		"undeclared @resource.extra on 1 of 2 series",
		"expected @resource.k8s.node.name on at least one of 2 series, found on none",
	}, bad.Problems(), "node keys removed from every series fail even though each series may lack them")
}

func TestTallySchemaChecksEveryHostType(t *testing.T) {
	series := func(hostType string, extra ...string) fakeSeries {
		l := map[string]string{
			"__name__": "m", "@resource.k8s.cluster.name": "c1",
			"@resource.host.type": hostType, "@resource.k8s.pod.name": "p-" + hostType,
		}
		for _, k := range extra {
			l["@resource."+k] = "x"
		}
		return fakeSeries{labels: l}
	}
	f, c := newFake(t, 100, []fakeSeries{series("a"), series("b", "leaked")})
	schema := GroupSchema{
		Resource: KeySchema{Required: []string{"k8s.cluster.name", "k8s.pod.name"}, Optional: []string{"host.type"}},
	}

	pinned := NewQueryCache(c, "c1", WithHostTypes([]string{"a"}))
	_, err := TallySchema(context.Background(), pinned, []string{"m"}, schema, nil)
	require.ErrorContains(t, err, "pass a cache without host types", "a pinned cache would hide the series of host type b")

	allNodes := NewQueryCache(c, "c1")
	tally, err := TallySchema(context.Background(), allNodes, []string{"m"}, schema, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"undeclared @resource.leaked on 1 of 2 series"}, tally.Problems())

	requests := len(f.queries())
	results, err := allNodes.Get(context.Background(), "m")
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Len(t, f.queries(), requests, "other every-series checks reuse the read of the schema row")
}
