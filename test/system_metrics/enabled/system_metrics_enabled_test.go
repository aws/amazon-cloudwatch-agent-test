// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package system_metrics_enabled

import (
	"fmt"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	namespace = "CWAgent/System"
	// Wait for batch flush (15 min) plus buffer for scrape + publish latency.
	agentRunDuration = 18 * time.Minute
	// Number of mock JVM agents to start.
	mockJVMCount = 2
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// startMockJVMs launches N mock JVM agents and returns their PIDs for cleanup.
func startMockJVMs(t *testing.T, count int) []string {
	t.Helper()
	var pids []string
	for i := 0; i < count; i++ {
		cmd := exec.Command("python3", "resources/mock_jvm.py")
		require.NoError(t, cmd.Start(), "failed to start mock JVM agent %d", i)
		pid := fmt.Sprintf("%d", cmd.Process.Pid)
		pids = append(pids, pid)
		log.Printf("Started mock JVM agent %d with PID %s", i, pid)
	}
	// Give sockets time to bind.
	time.Sleep(2 * time.Second)

	// Verify sockets appear in /proc/net/unix.
	for _, pid := range pids {
		out, err := exec.Command("bash", "-c", fmt.Sprintf("grep 'aws-jvm-metrics-%s' /proc/net/unix", pid)).Output()
		require.NoError(t, err, "socket for PID %s not found in /proc/net/unix", pid)
		require.Contains(t, string(out), "aws-jvm-metrics-"+pid)
		log.Printf("Verified socket @aws-jvm-metrics-%s in /proc/net/unix", pid)
	}
	return pids
}

// stopMockJVMs kills all mock JVM agent processes.
func stopMockJVMs(pids []string) {
	for _, pid := range pids {
		_ = exec.Command("kill", pid).Run()
	}
	// Also clean up any strays.
	_ = exec.Command("pkill", "-f", "mock_jvm.py").Run()
}

// getInstanceDimFilter returns a dimension filter for this instance's InstanceId.
func getInstanceDimFilter() []types.DimensionFilter {
	return []types.DimensionFilter{
		{
			Name:  aws.String("InstanceId"),
			Value: aws.String(awsservice.GetInstanceId()),
		},
	}
}

// getInstanceDims returns dimensions for metric data queries.
func getInstanceDims() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("InstanceId"),
			Value: aws.String(awsservice.GetInstanceId()),
		},
	}
}

// assertMetricExists verifies a metric exists in CloudWatch for this instance.
func assertMetricExists(t *testing.T, metricName string) {
	t.Helper()
	err := awsservice.ValidateMetric(metricName, namespace, getInstanceDimFilter())
	assert.NoError(t, err, "metric %s should exist in %s", metricName, namespace)
}

// assertMetricValue fetches metric values and asserts they are >= 0.
func assertMetricValue(t *testing.T, metricName string, startTime, endTime time.Time) {
	t.Helper()
	dims := getInstanceDims()
	data, err := awsservice.GetMetricStatistics(
		metricName, namespace, dims,
		startTime, endTime,
		900, // 15-min period matching batch interval
		[]types.Statistic{types.StatisticAverage},
		nil,
	)
	require.NoError(t, err, "failed to get statistics for %s", metricName)
	require.NotEmpty(t, data.Datapoints, "no datapoints for %s", metricName)
	for _, dp := range data.Datapoints {
		assert.GreaterOrEqual(t, *dp.Average, float64(0), "metric %s should be >= 0", metricName)
	}
}

// assertAggregateJVMCount verifies aggregate_jvm_count equals the expected count.
func assertAggregateJVMCount(t *testing.T, expected float64, startTime, endTime time.Time) {
	t.Helper()
	dims := getInstanceDims()
	data, err := awsservice.GetMetricStatistics(
		"aggregate_jvm_count", namespace, dims,
		startTime, endTime,
		900,
		[]types.Statistic{types.StatisticAverage},
		nil,
	)
	require.NoError(t, err, "failed to get statistics for aggregate_jvm_count")
	require.NotEmpty(t, data.Datapoints, "no datapoints for aggregate_jvm_count")
	assert.InDelta(t, expected, *data.Datapoints[0].Average, 0.5, "aggregate_jvm_count should be %v", expected)
}

func TestSystemMetricsEnabled(t *testing.T) {
	// Start mock JVM agents before the CWAgent.
	pids := startMockJVMs(t, mockJVMCount)
	defer stopMockJVMs(pids)

	// Start agent. The JSON config has system_metrics_enabled: true.
	common.CopyFile("resources/agent_config.json", common.ConfigOutputPath)
	err := common.StartAgent(common.ConfigOutputPath, true, false)
	require.NoError(t, err, "agent failed to start")

	startTime := time.Now()
	log.Printf("Agent started, waiting %s for batch flush...", agentRunDuration)
	time.Sleep(agentRunDuration)
	common.StopAgent()
	endTime := time.Now()

	log.Printf("Agent stopped. Validating metrics in %s...", namespace)

	// --- JVM per-JVM heap metrics ---
	jvmMetrics := []string{"heap_max_bytes", "heap_committed_bytes", "heap_after_gc_bytes", "heap_free_after_gc_bytes"}
	for _, m := range jvmMetrics {
		m := m
		t.Run("JVM/"+m, func(t *testing.T) {
			assertMetricExists(t, m)
			assertMetricValue(t, m, startTime, endTime)
		})
	}

	// --- JVM aggregate metrics ---
	t.Run("JVM/aggregate_jvm_count", func(t *testing.T) {
		assertAggregateJVMCount(t, float64(mockJVMCount), startTime, endTime)
	})
	jvmAggMetrics := []string{"aggregate_heap_max_bytes", "aggregate_heap_free_after_gc_bytes", "aggregate_heap_after_gc_utilized"}
	for _, m := range jvmAggMetrics {
		m := m
		t.Run("JVM/"+m, func(t *testing.T) {
			assertMetricExists(t, m)
			assertMetricValue(t, m, startTime, endTime)
		})
	}

	// --- System metrics (cpu, mem, disk, ena) ---
	systemMetrics := []string{
		"cpu_time_iowait",
		"mem_total", "mem_available", "mem_cached", "mem_active",
		"aggregate_disk_used", "aggregate_disk_free",
		"aggregate_bw_in_allowance_exceeded", "aggregate_bw_out_allowance_exceeded", "aggregate_pps_allowance_exceeded",
	}
	for _, m := range systemMetrics {
		m := m
		t.Run("System/"+m, func(t *testing.T) {
			assertMetricExists(t, m)
			assertMetricValue(t, m, startTime, endTime)
		})
	}
}
