//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	checkpointTargetDeploy = "deploy/nginx-test"
	checkpointTargetNS     = "default"
	checkpointTotalMarkers = 20
	checkpointKillAfter    = 10
)

func TestCheckpointRestartRecovery(t *testing.T) {
	ctx := context.Background()
	runID := fmt.Sprintf("ckpt-%d", time.Now().UnixNano()%1000000)

	podInfo, err := kubectlRun("get", "pod", "-n", checkpointTargetNS,
		"-l", "app=nginx-test",
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={.items[0].metadata.name},{.items[0].spec.nodeName}")
	require.NoError(t, err, "finding nginx-test pod")
	parts := strings.SplitN(strings.TrimSpace(podInfo), ",", 2)
	require.Len(t, parts, 2, "expected pod,node from nginx-test")
	nginxPod, nginxNode := parts[0], parts[1]
	t.Logf("nginx-test pod: %s on node: %s", nginxPod, nginxNode)

	agentPod, err := kubectlRun("get", "pod", "-n", "amazon-cloudwatch",
		"-l", "app.kubernetes.io/name=cloudwatch-agent",
		"--field-selector=spec.nodeName="+nginxNode+",status.phase=Running",
		"-o", "jsonpath={.items[0].metadata.name}")
	require.NoError(t, err, "finding agent pod")
	agentPod = strings.TrimSpace(agentPod)
	require.NotEmpty(t, agentPod, "no agent pod on node %s", nginxNode)
	t.Logf("agent pod: %s", agentPod)

	// Step 1: Warmup — confirm agent is tailing nginx-test.
	warmup := fmt.Sprintf("%s-warmup", runID)
	emitMarkerToNginx(t, warmup)
	t.Log("warmup emitted, waiting 2 min")
	time.Sleep(2 * time.Minute)

	count := queryMarkerCount(t, ctx, warmup, 5*time.Minute)
	require.True(t, count > 0, "warmup marker not in CW Logs — agent not tailing nginx-test")
	t.Log("warmup confirmed")

	// Step 2: Emit first batch.
	t.Logf("emitting markers 1-%d", checkpointKillAfter)
	for i := 1; i <= checkpointKillAfter; i++ {
		emitMarkerToNginx(t, fmt.Sprintf("%s-marker-%03d", runID, i))
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(5 * time.Second)

	// Step 3: Kill agent.
	t.Logf("killing agent pod %s", agentPod)
	_, err = kubectlRun("delete", "pod", "-n", "amazon-cloudwatch", agentPod, "--grace-period=0", "--force")
	require.NoError(t, err)
	time.Sleep(3 * time.Second)

	// Step 4: Emit remaining markers while agent is down.
	t.Logf("emitting markers %d-%d (agent restarting)", checkpointKillAfter+1, checkpointTotalMarkers)
	for i := checkpointKillAfter + 1; i <= checkpointTotalMarkers; i++ {
		emitMarkerToNginx(t, fmt.Sprintf("%s-marker-%03d", runID, i))
		time.Sleep(300 * time.Millisecond)
	}

	// Step 5: Wait for restart.
	t.Log("waiting for agent restart")
	require.Eventually(t, func() bool {
		out, _ := kubectlRun("get", "pod", "-n", "amazon-cloudwatch",
			"-l", "app.kubernetes.io/name=cloudwatch-agent",
			"--field-selector=spec.nodeName="+nginxNode+",status.phase=Running",
			"-o", "jsonpath={.items[0].metadata.name}")
		name := strings.TrimSpace(out)
		return name != "" && name != agentPod
	}, 120*time.Second, 5*time.Second)

	t.Log("waiting 4 min for propagation")
	time.Sleep(4 * time.Minute)

	// Step 6: Verify all markers.
	seen := make(map[int]int)
	for i := 1; i <= checkpointTotalMarkers; i++ {
		seen[i] = queryMarkerCount(t, ctx, fmt.Sprintf("%s-marker-%03d", runID, i), 15*time.Minute)
	}

	t.Log("results:")
	for i := 1; i <= checkpointTotalMarkers; i++ {
		t.Logf("  marker-%03d: %d", i, seen[i])
	}

	var missing []int
	for i := 1; i <= checkpointTotalMarkers; i++ {
		if seen[i] == 0 {
			missing = append(missing, i)
		}
	}
	require.Empty(t, missing, "GAPS — markers not delivered: %v", missing)

	var dupes []string
	for i := 1; i <= checkpointTotalMarkers; i++ {
		if seen[i] > 1 {
			dupes = append(dupes, fmt.Sprintf("%03d(x%d)", i, seen[i]))
		}
	}
	require.Empty(t, dupes, "DUPLICATES: %v", dupes)

	t.Logf("PASS: all %d markers delivered exactly once", checkpointTotalMarkers)
}

func emitMarkerToNginx(t *testing.T, msg string) {
	t.Helper()
	_, err := kubectlRun("exec", "-n", checkpointTargetNS, checkpointTargetDeploy, "--",
		"sh", "-c", fmt.Sprintf("echo '%s' >> /proc/1/fd/1", msg))
	require.NoError(t, err, "emitting %q", msg)
}

func queryMarkerCount(t *testing.T, ctx context.Context, marker string, lookback time.Duration) int {
	t.Helper()
	query := fmt.Sprintf(`fields @message | filter @message like '%s' | limit 100`, marker)
	results, err := logsClient.QueryRaw(ctx, appLogGroup(), query, lookback)
	require.NoError(t, err, "querying %q", marker)
	return len(results)
}

func kubectlRun(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
