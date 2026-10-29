//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestKSMAvailabilityZoneIsTheObjectsZone — the cluster scraper runs on one
// node, so its own AZ is wrong for most kube-state-metrics series. On every KSM
// series, cloud.availability_zone must be the zone of the node the series names:
// nodemetadataenricher sets it, or else the node's topology.kubernetes.io/zone label
// does, and a pod's own zone label replaces it. A series whose node k8sattributes
// knows must carry it. A series that names no node, such as a Deployment's, must not.
// The pod zone label itself is denied, so the API is the reference for it.
// Series are read cluster-wide, not through queryCache.Get, whose host.type=""
// filter on workload metrics would hide a series that gained the scraper's host keys.
// ---------------------------------------------------------------------------

const zoneLabel = "topology.kubernetes.io/zone"

func TestKSMAvailabilityZoneIsTheObjectsZone(t *testing.T) {
	gt := freshGroundTruth(t)
	buckets := map[string][]string{
		"node": ksmNodeBucket, "pod": ksmPodBucket, "container": ksmContainerBucket,
		"object": otelmetrics.ConcatKeys(ksmWorkloadBucket.Deployment, ksmWorkloadBucket.DaemonSet,
			ksmWorkloadBucket.StatefulSet, ksmWorkloadBucket.Job, ksmWorkloadBucket.CronJob, ksmClusterBucket),
	}
	for _, bucket := range []string{"node", "pod", "container", "object"} {
		bucket := bucket
		t.Run(bucket, func(t *testing.T) {
			var series, withZone, fromPodLabel int
			wrong, missing, absent, unexpected := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
			for _, metricName := range buckets[bucket] {
				for _, r := range queryAllSeries(t, metricName) {
					series++
					res := r.Labels.Resource
					az, hasZone := res[ksmZoneKey]
					if hasZone {
						withZone++
					}
					nodeName := res["k8s.node.name"]
					if nodeName == "" {
						if hasZone {
							unexpected[metricName]++
						}
						continue
					}
					node, known := gt.nodes[nodeName]
					if !known {
						continue // the node left between the API read and the scrape
					}
					if hasZone && az != node.Labels[zoneLabel] {
						wrong[metricName+": "+az+" on a node in "+node.Labels[zoneLabel]]++
					}
					pod, ok := gt.pods[res["k8s.namespace.name"]+"/"+res["k8s.pod.name"]]
					if want := pod.Labels[zoneLabel]; ok && want != "" && pod.Spec.NodeName == nodeName {
						fromPodLabel++
						if az != want {
							missing[metricName]++
						}
					} else if !hasZone && node.Labels[zoneLabel] != "" && res["k8s.node.uid"] != "" {
						absent[metricName]++ // transform/cw_k8s_ci_v0_ksm_zone takes the node's zone label
					}
				}
			}
			t.Logf("%d series, %d with an AZ, %d of a pod with a zone label", series, withZone, fromPodLabel)
			require.NotZero(t, series, "no %s series", bucket)
			for _, k := range otelmetrics.SortedKeys(wrong) {
				t.Errorf("%s: wrong AZ on %d series", k, wrong[k])
			}
			for _, k := range otelmetrics.SortedKeys(missing) {
				t.Errorf("%s: %d series of a pod with a zone label lack that AZ", k, missing[k])
			}
			for _, k := range otelmetrics.SortedKeys(absent) {
				t.Errorf("%s: %d series of a known node with a zone label carry no AZ", k, absent[k])
			}
			for _, k := range otelmetrics.SortedKeys(unexpected) {
				t.Errorf("%s: %d series name no node but carry an AZ, the cluster scraper's", k, unexpected[k])
			}
			if bucket == "node" || bucket == "pod" {
				require.NotZero(t, withZone, "no %s series carries an AZ", bucket)
			}
		})
	}
}
