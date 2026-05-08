//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ebs_csi

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// TestEBSCSIPrerequisites verifies the EBS test infrastructure exists.
func TestEBSCSIPrerequisites(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	t.Run("pvc_exists", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, p := range gt.pods {
			p := p
			for _, v := range p.Spec.Volumes {
				v := v
				if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == "ebs-test-pvc" {
					found = true
					break
				}
			}
		}
		require.True(t, found,
			"no pod is mounting ebs-test-pvc — the ebs-test Deployment may have been deleted")
	})

	t.Run("pod_running", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, p := range gt.pods {
			p := p
			if p.Labels["app"] == "ebs-test" && p.Status.Phase == corev1.PodRunning {
				found = true
				break
			}
		}
		require.True(t, found,
			"no running ebs-test pod found — EBS CSI metrics require a pod with an attached EBS volume")
	})
}

// TestEBSCSIInstrumentation verifies instrumentation source for EBS CSI metrics.
func TestEBSCSIInstrumentation(t *testing.T) {
	t.Parallel()
	for _, metricName := range ebsCsiMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopePrometheus, name, "%s instrumentation name", metricName)
			}
		})
	}
}

// TestEBSCSIVolumeId verifies volume_id is promoted to resource scope.
func TestEBSCSIVolumeId(t *testing.T) {
	t.Parallel()
	for _, metricName := range ebsCsiMetricNames() {
		metricName := metricName
		t.Run(metricName+"/volume_id", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				volID, ok := r.Labels.Resource["volume_id"]
				require.True(t, ok, "%s missing @resource.volume_id", metricName)
				require.True(t, strings.HasPrefix(volID, "vol-"),
					"%s volume_id should start with 'vol-', got '%s'", metricName, volID)
			}
		})
	}
}

// TestEBSCSINoHwLabels verifies hw.* labels are absent.
func TestEBSCSINoHwLabels(t *testing.T) {
	t.Parallel()
	hwAttrs := []string{"hw.type", "hw.vendor", "hw.id"}
	for _, metricName := range ebsCsiMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				for _, attr := range hwAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.False(t, has,
						"%s should not have @resource.%s", metricName, attr)
				}
			}
		})
	}
}

// TestEBSCSIInstanceId verifies instance_id at resource, absent from datapoint.
func TestEBSCSIInstanceId(t *testing.T) {
	t.Parallel()
	for _, metricName := range ebsCsiMetricNames() {
		metricName := metricName
		t.Run(metricName+"/resource_present", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				instID, ok := r.Labels.Resource["instance_id"]
				require.True(t, ok, "%s missing @resource.instance_id", metricName)
				require.True(t, strings.HasPrefix(instID, "i-"),
					"%s instance_id should start with 'i-', got '%s'", metricName, instID)
			}
		})
		t.Run(metricName+"/datapoint_absent", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Datapoint["instance_id"]
				require.False(t, has,
					"%s should not have datapoint instance_id after promotion", metricName)
			}
		})
		t.Run(metricName+"/volume_id_datapoint_absent", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EBS CSI driver?)", metricName)
			for _, r := range results {
				r := r
				_, has := r.Labels.Datapoint["volume_id"]
				require.False(t, has,
					"%s should not have datapoint volume_id after promotion", metricName)
			}
		})
	}
}
