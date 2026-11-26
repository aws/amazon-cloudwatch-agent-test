// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package load

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	eksClusterName = flag.String("eksClusterName", "", "EKS cluster name under test")
	podCount       = flag.Int("podCount", 100, "generator pod count")
	sampleWindow   = flag.Duration("sampleWindow", 3*time.Minute, "measurement window per mode")
	warmup         = flag.Duration("warmup", 60*time.Second, "warm-up after a mode switch before the window")
)

type thresholds struct {
	MaxP99Ratio                       float64 `json:"max_p99_ratio"`
	MaxThrottleDeltaPerSec            float64 `json:"max_throttle_delta_per_sec"`
	MaxFlowcontrolRejectedDeltaPerSec float64 `json:"max_flowcontrol_rejected_delta_per_sec"`
	MinEnrichedFraction               float64 `json:"min_enriched_fraction"`
}

func readThresholds(t *testing.T) thresholds {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("resources", "load_thresholds.json"))
	require.NoError(t, err)
	var th thresholds
	require.NoError(t, json.Unmarshal(data, &th))
	return th
}

func TestUseKubeletMetadataLoad(t *testing.T) {
	require.NotEmpty(t, *eksClusterName, "-eksClusterName is required")
	th := readThresholds(t)
	cs := getClients(t)

	waitForGenerator(t, cs, *podCount)

	for _, mode := range []string{"On", "Off"} {
		t.Run(mode, func(t *testing.T) {
			setFluentBitKubeletMode(t, cs, mode)
			time.Sleep(*warmup)

			start := time.Now()
			time.Sleep(*sampleWindow)
			end := time.Now()

			frac := fractionEnriched(t, *eksClusterName)
			t.Logf("[Use_Kubelet=%s] enriched fraction=%.4f (min %.4f)", mode, frac, th.MinEnrichedFraction)
			require.GreaterOrEqualf(t, frac, th.MinEnrichedFraction,
				"metadata enrichment below threshold with Use_Kubelet=%s", mode)

			reqs := apiserverRequestsByFluentBit(t, *eksClusterName, start, end)
			secs := (*sampleWindow).Seconds()
			t.Logf("[Use_Kubelet=%s] fluent-bit apiserver pod/node requests (audit)=%d over %.0fs (%.3f/s)",
				mode, reqs, secs, float64(reqs)/secs)
		})
	}
}
