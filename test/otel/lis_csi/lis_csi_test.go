//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package lis_csi

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLISCSIPrerequisites(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	t.Run("pod_running", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, p := range gt.pods {
			p := p
			if p.Labels["app"] == "liscsi-integ-test" && p.Status.Phase == "Running" {
				found = true
				break
			}
		}
		require.True(t, found, "no running liscsi-integ-test pod found")
	})
}

func TestLISCSIInstrumentation(t *testing.T) {
	t.Parallel()
	for _, metricName := range lisCsiMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopePrometheus, name, "%s instrumentation name", metricName)
			}
		})
	}
}

func TestLISCSIVolumeId(t *testing.T) {
	t.Parallel()
	for _, metricName := range lisCsiVolumeMetricNames() {
		metricName := metricName
		t.Run(metricName+"/volume_id", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			for _, r := range results {
				r := r
				volID, ok := r.Labels.Resource["volume_id"]
				require.True(t, ok, "%s missing @resource.volume_id", metricName)
				require.True(t, strings.HasPrefix(volID, "pvc-"), "%s volume_id should start with 'pvc-', got '%s'", metricName, volID)
			}
		})
	}
}

func TestLISCSINoHwLabels(t *testing.T) {
	t.Parallel()
	hwAttrs := []string{"hw.type", "hw.vendor", "hw.id"}
	for _, metricName := range lisCsiMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			for _, r := range results {
				r := r
				for _, attr := range hwAttrs {
					attr := attr
					_, has := r.Labels.Resource[attr]
					require.True(t, !has, "%s should not have @resource.%s", metricName, attr)
				}
			}
		})
	}
}

func TestLISCSIInstanceId(t *testing.T) {
	t.Parallel()
	for _, metricName := range lisCsiVolumeMetricNames() {
		metricName := metricName

		t.Run(metricName+"/resource_present", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			for _, r := range results {
				r := r
				instID, ok := r.Labels.Resource["instance_id"]
				require.True(t, ok, "%s missing @resource.instance_id", metricName)
				require.True(t, strings.HasPrefix(instID, "i-"), "%s instance_id should start with 'i-', got '%s'", metricName, instID)
			}
		})

		t.Run(metricName+"/datapoint_absent", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			for _, r := range results {
				r := r
				_, has := r.Labels.Datapoint["instance_id"]
				require.True(t, !has, "%s should not have datapoint instance_id", metricName)
			}
		})

		t.Run(metricName+"/volume_id_datapoint_absent", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			for _, r := range results {
				r := r
				_, has := r.Labels.Datapoint["volume_id"]
				require.True(t, !has, "%s should not have datapoint volume_id", metricName)
			}
		})
	}
}
