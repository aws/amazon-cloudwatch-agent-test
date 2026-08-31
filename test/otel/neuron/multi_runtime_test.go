//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Multi-runtime Neuron tests.
//
// On a node with >1 Neuron runtime, the pre-fix agent replaced half of every
// per-core metric with zero: the promote ran in `context: datapoint` but wrote
// resource attributes (per-ResourceMetrics, so last write won) and deleted
// runtime_tag, collapsing two datapoints onto one identity. Option A adds
// runtime_tag to the groupbyattrs keys instead.
//
// The existing tests miss it. TestNeuronRuntimeTagInResourceScope passes on the
// broken code (the collapsed resource still carries one tag), and
// TestNeuronNoDuplicateSeries passes because the collision happens in-agent, so
// the surface shows too FEW series rather than duplicates.
//
// Fixture: terraform/eks/daemon/otel-neuron/main.tf co-locates neuron-burn-core
// and neuron-burn-peer on one inf2.xlarge (1 device x 2 cores), each holding
// aws.amazon.com/neuroncore: "1".
//
// Label semantics, easy to get backwards: k8s.pod.name is the pod that OWNS THE
// CORE (awsdevicepodcorrelation maps core -> pod); runtime.tag is the runtime that
// PRODUCED THE READING. They cross — runtime B emits a zero for core 0, correlated
// to pod A. So core -> pod is 1:1; tag -> pod is not.

package neuron

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const runtimeTagResourceKey = "aws.neuron.runtime.tag"

// What the in-agent aggregation alternatives stamp instead of a real tag.
const runtimeTagFlattenedSentinel = "DEFAULT"

const (
	burnCorePodPrefix = "neuron-burn-core"
	burnPeerPodPrefix = "neuron-burn-peer"
)

const fixtureRequirement = "the otel-neuron terraform fixture must have " +
	"neuron-burn-core AND neuron-burn-peer Running and co-located on one " +
	"inf2.xlarge node, each holding aws.amazon.com/neuroncore: \"1\""

type coreIdentity struct {
	node   string
	device string
	core   string
}

func (c coreIdentity) String() string {
	return fmt.Sprintf("%s device=%s core=%s", c.node, c.device, c.core)
}

// ok is false for runtime-level metrics, which have no core dimension.
func coreIdentityOf(r otelmetrics.MetricResult) (coreIdentity, bool) {
	node := r.Labels.Resource["k8s.node.name"]
	device, hasDevice := r.Labels.Datapoint["aws.neuron.device"]
	core, hasCore := r.Labels.Datapoint["aws.neuron.core"]
	if node == "" || !hasDevice || !hasCore {
		return coreIdentity{}, false
	}
	return coreIdentity{node: node, device: device, core: core}, true
}

func runtimeTagsByNode(results []otelmetrics.MetricResult) map[string]map[string]struct{} {
	byNode := make(map[string]map[string]struct{})
	for _, r := range results {
		r := r
		node := r.Labels.Resource["k8s.node.name"]
		tag := r.Labels.Resource[runtimeTagResourceKey]
		if node == "" || tag == "" {
			continue
		}
		if byNode[node] == nil {
			byNode[node] = make(map[string]struct{})
		}
		byNode[node][tag] = struct{}{}
	}
	return byNode
}

func multiRuntimeNodes(results []otelmetrics.MetricResult) []string {
	var nodes []string
	for node, tags := range runtimeTagsByNode(results) {
		if len(tags) >= 2 {
			nodes = append(nodes, node)
		}
	}
	sort.Strings(nodes)
	return nodes
}

func resultsForNode(results []otelmetrics.MetricResult, node string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		r := r
		if r.Labels.Resource["k8s.node.name"] == node {
			out = append(out, r)
		}
	}
	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedTags(results []otelmetrics.MetricResult) []string {
	set := make(map[string]struct{})
	for _, r := range results {
		r := r
		if tag := r.Labels.Resource[runtimeTagResourceKey]; tag != "" {
			set[tag] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func tagSummary(byNode map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(byNode))
	for node, tags := range byNode {
		out[node] = sortedKeys(tags)
	}
	return out
}

// Hard-fails rather than skipping: the fixture is part of the cluster definition,
// and a skip would silently void the whole regression.
//
// The message names BOTH causes deliberately. Verified against a reverted agent:
// the collapse also loses the runtime tag itself, so only one tag survives per node
// and this guard fires before any specific assertion. A single tag is structurally
// indistinguishable from a genuine single-runtime node (zeus-poc-cluster looks
// identical), so the surface cannot tell them apart — the reader has to.
func multiRuntimeResults(t *testing.T, metricName string) ([]otelmetrics.MetricResult, []string) {
	t.Helper()
	results, err := queryCache.Get(context.Background(), metricName)
	require.NoError(t, err, "querying %s", metricName)
	require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)

	nodes := multiRuntimeNodes(results)
	require.NotEmpty(t, nodes,
		"%s: no node reports >= 2 distinct @resource.%s. Observed tags per node: %v.\n"+
			"TWO causes look identical here, check both:\n"+
			"  1. THE DEFECT — the agent collapsed the runtime dimension, so one tag "+
			"overwrote the others and half the per-core values were replaced by zero. "+
			"Confirm with `kubectl -n amazon-cloudwatch get cm cloudwatch-agent -o yaml`: "+
			"groupbyattrs/cw_k8s_ci_v0_neuron must include the runtime_tag key, and "+
			"transform/cw_k8s_ci_v0_neuron_promote must run in `context: resource`.\n"+
			"  2. THE FIXTURE — only one Neuron runtime is running, so there is nothing "+
			"to collapse. %s",
		metricName, runtimeTagResourceKey, tagSummary(runtimeTagsByNode(results)), fixtureRequirement)
	return results, nodes
}

// TestMultiRuntimeDistinctWorkloadsOwnDistinctCores is the value-level detector:
// two workloads burning two cores must yield two non-zero readings. Pre-fix there
// was one, the other having been overwritten by a second runtime's zero.
func TestMultiRuntimeDistinctWorkloadsOwnDistinctCores(t *testing.T) {
	t.Parallel()
	const metricName = "neuroncore_utilization_ratio"
	results, nodes := multiRuntimeResults(t, metricName)

	for _, node := range nodes {
		node := node
		t.Run(node, func(t *testing.T) {
			t.Parallel()
			busyCores := make(map[coreIdentity]struct{})
			busyTags := make(map[string]struct{})
			var detail []string

			for _, r := range resultsForNode(results, node) {
				r := r
				if r.Value <= 0 {
					continue
				}
				id, ok := coreIdentityOf(r)
				if !ok {
					continue
				}
				busyCores[id] = struct{}{}
				if tag := r.Labels.Resource[runtimeTagResourceKey]; tag != "" {
					busyTags[tag] = struct{}{}
				}
				detail = append(detail, fmt.Sprintf("core=%s tag=%s pod=%s value=%.4f",
					id.core, r.Labels.Resource[runtimeTagResourceKey],
					r.Labels.Resource["k8s.pod.name"], r.Value))
			}
			sort.Strings(detail)

			require.GreaterOrEqual(t, len(busyCores), 2,
				"%s on %s: expected >= 2 cores with non-zero utilization, got %d. Exactly "+
					"one busy core is the signature of the per-core data-loss defect. "+
					"Non-zero series: %v. %s",
				metricName, node, len(busyCores), detail, fixtureRequirement)

			// Deliberately no assertion on distinct POD count. Two runtimes need not be
			// two pods -- solstice-gpu-test runs both from one pod -- so requiring it
			// encodes the terraform fixture's shape rather than the invariant.
			require.GreaterOrEqual(t, len(busyTags), 2,
				"%s on %s: %d busy cores attributed to only %d runtime tag(s) %v — the "+
					"runtime dimension collapsed. Non-zero series: %v",
				metricName, node, len(busyCores), len(busyTags), sortedKeys(busyTags), detail)
		})
	}
}

// TestMultiRuntimeFixtureIsBothBurnWorkloads fails loudly if neuron-burn-peer is
// removed, rather than letting the tests above become vacuous.
func TestMultiRuntimeFixtureIsBothBurnWorkloads(t *testing.T) {
	t.Parallel()
	const metricName = "neuroncore_utilization_ratio"
	results, nodes := multiRuntimeResults(t, metricName)

	for _, node := range nodes {
		node := node
		t.Run(node, func(t *testing.T) {
			t.Parallel()
			pods := make(map[string]struct{})
			for _, r := range resultsForNode(results, node) {
				r := r
				if pod := r.Labels.Resource["k8s.pod.name"]; pod != "" {
					pods[pod] = struct{}{}
				}
			}
			names := sortedKeys(pods)

			var sawCore, sawPeer bool
			for _, p := range names {
				if strings.HasPrefix(p, burnPeerPodPrefix) {
					sawPeer = true
				} else if strings.HasPrefix(p, burnCorePodPrefix) {
					sawCore = true
				}
			}
			require.True(t, sawCore,
				"no pod prefixed %q among %v on multi-runtime node %s", burnCorePodPrefix, names, node)
			require.True(t, sawPeer,
				"no pod prefixed %q among %v on multi-runtime node %s. %s",
				burnPeerPodPrefix, names, node, fixtureRequirement)
		})
	}
}

// TestMultiRuntimeCoreCrossProductPreserved is the structural detector: every core
// is reported by every runtime, so series must equal cores x tags. Catches the
// collapse even on an idle node, where values cannot distinguish it.
func TestMultiRuntimeCoreCrossProductPreserved(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, nodes := multiRuntimeResults(t, md.Name)

			for _, node := range nodes {
				node := node
				nodeResults := resultsForNode(results, node)
				tags := sortedTags(nodeResults)

				cores := make(map[coreIdentity]struct{})
				pairs := make(map[string]struct{})
				for _, r := range nodeResults {
					r := r
					id, ok := coreIdentityOf(r)
					if !ok {
						continue
					}
					cores[id] = struct{}{}
					pairs[fmt.Sprintf("%s|%s", id, r.Labels.Resource[runtimeTagResourceKey])] = struct{}{}
				}

				require.NotEmpty(t, cores, "%s: no core-level results on node %s", md.Name, node)
				want := len(cores) * len(tags)
				require.Equal(t, want, len(pairs),
					"%s on %s: expected %d (core x runtime-tag) series for %d cores x %d "+
						"runtimes %v, got %d. Fewer means one runtime's datapoints are "+
						"shadowing another's.",
					md.Name, node, want, len(cores), len(tags), tags, len(pairs))
			}
		})
	}
}

// TestMultiRuntimeCoreIdentityIsUnique narrows TestNeuronNoDuplicateSeries to the
// tuple that must be unique, so dropping one of these attributes cannot pass
// because some unrelated label still differed.
func TestMultiRuntimeCoreIdentityIsUnique(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, nodes := multiRuntimeResults(t, md.Name)

			for _, node := range nodes {
				node := node
				seen := make(map[string]int)
				for _, r := range resultsForNode(results, node) {
					r := r
					id, ok := coreIdentityOf(r)
					if !ok {
						continue
					}
					seen[fmt.Sprintf("%s|tag=%s", id, r.Labels.Resource[runtimeTagResourceKey])]++
				}
				for key, count := range seen {
					require.Equal(t, 1, count,
						"%s on %s: %d series share the identity %s", md.Name, node, count, key)
				}
			}
		})
	}
}

// TestMultiRuntimeOneRuntimeOwnsEachCore is the mirror image: one runtime holds a
// core, so two non-zero readings for one core means shared core identity.
func TestMultiRuntimeOneRuntimeOwnsEachCore(t *testing.T) {
	t.Parallel()
	const metricName = "neuroncore_utilization_ratio"
	results, nodes := multiRuntimeResults(t, metricName)

	for _, node := range nodes {
		node := node
		t.Run(node, func(t *testing.T) {
			t.Parallel()
			nonZeroPerCore := make(map[coreIdentity][]string)
			for _, r := range resultsForNode(results, node) {
				r := r
				if r.Value <= 0 {
					continue
				}
				id, ok := coreIdentityOf(r)
				if !ok {
					continue
				}
				nonZeroPerCore[id] = append(nonZeroPerCore[id],
					r.Labels.Resource[runtimeTagResourceKey])
			}
			for id, tags := range nonZeroPerCore {
				sort.Strings(tags)
				require.LessOrEqual(t, len(tags), 1,
					"%s on %s: %d runtime tags %v report non-zero for %s",
					metricName, node, len(tags), tags, id)
			}
		})
	}
}

// TestMultiRuntimeCoreToPodIsOneToOne is an invariant guard, not a regression
// detector — core -> pod stayed 1:1 pre-fix. It is here because the tempting
// version (tag -> pod is 1:1) fails on correct code; see the header.
func TestMultiRuntimeCoreToPodIsOneToOne(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, nodes := multiRuntimeResults(t, md.Name)

			for _, node := range nodes {
				node := node
				coreToPods := make(map[coreIdentity]map[string]struct{})
				for _, r := range resultsForNode(results, node) {
					r := r
					id, ok := coreIdentityOf(r)
					if !ok {
						continue
					}
					pod := r.Labels.Resource["k8s.pod.name"]
					if pod == "" {
						continue
					}
					if coreToPods[id] == nil {
						coreToPods[id] = make(map[string]struct{})
					}
					coreToPods[id][pod] = struct{}{}
				}
				for id, pods := range coreToPods {
					require.Equal(t, 1, len(pods),
						"%s on %s: %s is attributed to %d pods %v",
						md.Name, node, id, len(pods), sortedKeys(pods))
				}
			}
		})
	}
}

// TestMultiRuntimeTagNotFlattened is a tripwire: the in-agent aggregation
// alternatives fix the zeros but stamp "DEFAULT", losing per-runtime attribution.
// Changing that trade-off should mean changing this test.
func TestMultiRuntimeTagNotFlattened(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)

			for _, r := range results {
				r := r
				tag, ok := r.Labels.Resource[runtimeTagResourceKey]
				if !ok {
					continue
				}
				require.NotEqual(t, runtimeTagFlattenedSentinel, tag,
					"%s carries @resource.%s=%q: the runtime dimension was aggregated away "+
						"in-agent, losing per-runtime attribution",
					metricName, runtimeTagResourceKey, tag)
			}
		})
	}
}

// TestMultiRuntimeTagAbsentFromDatapoint catches the groupbyattrs key being
// removed, which undoes the fix even while values still look right under light load.
func TestMultiRuntimeTagAbsentFromDatapoint(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, nodes := multiRuntimeResults(t, md.Name)

			for _, node := range nodes {
				node := node
				for _, r := range resultsForNode(results, node) {
					r := r
					_, hasDatapointTag := r.Labels.Datapoint["runtime_tag"]
					require.False(t, hasDatapointTag,
						"%s on %s: runtime_tag is still a datapoint attribute (%q)",
						md.Name, node, r.Labels.Datapoint["runtime_tag"])

					tag, hasResourceTag := r.Labels.Resource[runtimeTagResourceKey]
					require.True(t, hasResourceTag,
						"%s on %s: missing @resource.%s", md.Name, node, runtimeTagResourceKey)
					require.NotEmpty(t, tag,
						"%s on %s: @resource.%s is empty", md.Name, node, runtimeTagResourceKey)
				}
			}
		})
	}
}
