// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package system_metrics_disabled

import (
	"log"
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
	// With force_flush_interval=60s in test config, 2min is enough to prove nothing leaks.
	agentRunDuration = 2 * time.Minute
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// systemMetricNames is the full set of metrics our receiver would publish.
var systemMetricNames = []string{
	// JVM
	"heap_max_bytes", "heap_committed_bytes", "heap_after_gc_bytes", "heap_free_after_gc_bytes",
	"aggregate_jvm_count", "aggregate_heap_max_bytes", "aggregate_heap_free_after_gc_bytes",
	"aggregate_heap_after_gc_utilized",
	// System
	"cpu_time_iowait", "mem_total", "mem_available", "mem_cached", "mem_active",
	"aggregate_disk_used", "aggregate_disk_free",
	"aggregate_bw_in_allowance_exceeded", "aggregate_bw_out_allowance_exceeded",
	"aggregate_pps_allowance_exceeded",
}

func TestSystemMetricsDisabled(t *testing.T) {
	common.CopyFile("resources/agent_config.json", common.ConfigOutputPath)
	err := common.StartAgent(common.ConfigOutputPath, true, false)
	require.NoError(t, err, "agent failed to start")

	log.Printf("Agent started, waiting %s to confirm no system metrics are published...", agentRunDuration)
	time.Sleep(agentRunDuration)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()
	dimFilter := []types.DimensionFilter{
		{Name: aws.String("InstanceId"), Value: aws.String(instanceId)},
	}

	// Verify no system metrics exist for THIS instance in CWAgent/System.
	// The namespace may exist from other test runs, but no metrics should have our InstanceId.
	for _, metricName := range systemMetricNames {
		t.Run("Absent/"+metricName, func(t *testing.T) {
			err := awsservice.ValidateMetric(metricName, namespace, dimFilter)
			assert.Error(t, err, "metric %s should NOT exist for instance %s in %s",
				metricName, instanceId, namespace)
		})
	}
}
