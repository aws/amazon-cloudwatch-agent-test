// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package localtest_host_metrics

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const namespace = "LocalTestHostMetrics"

var metricCategories = map[string][]string{
	"cpu":       {"cpu_usage_active", "cpu_usage_idle"},
	"mem":       {"mem_used_percent", "mem_total"},
	"disk":      {"disk_used_percent", "disk_free"},
	"diskio":    {"diskio_reads", "diskio_writes"},
	"net":       {"net_bytes_sent", "net_bytes_recv"},
	"netstat":   {"netstat_tcp_established", "netstat_tcp_time_wait"},
	"swap":      {"swap_free", "swap_used_percent"},
	"processes": {"processes_running", "processes_total"},
	"procstat":  {"procstat_cpu_usage", "procstat_memory_rss"},
}

var hostMetricConfigs = []string{
	"cpu_config.json",
	"mem_config.json",
	"disk_config.json",
	"diskio_config.json",
	"net_config.json",
	"netstat_config.json",
	"swap_config.json",
	"processes_config.json",
	"procstat_config.json",
}

func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", start)
		}
		dir = parent
	}
}

func generateMergedConfig(repoRoot string, outputPath string) error {
	script := filepath.Join(repoRoot, "scripts", "merge-agent-configs.sh")
	args := []string{script, "-n", namespace, "-o", outputPath}
	configDir := filepath.Join(repoRoot, "test", "metric_value_benchmark", "agent_configs")
	for _, cfg := range hostMetricConfigs {
		args = append(args, filepath.Join(configDir, cfg))
	}
	cmd := exec.Command("bash", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestHostMetrics(t *testing.T) {
	_, testFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(testFile)
	staticConfig := filepath.Join(testDir, "agent_config.json")

	configPath := staticConfig
	if repoRoot, err := findRepoRoot(testDir); err == nil {
		tmpFile, err := os.CreateTemp("", "merged-agent-config-*.json")
		if err == nil {
			tmpPath := tmpFile.Name()
			tmpFile.Close()
			if err := generateMergedConfig(repoRoot, tmpPath); err != nil {
				log.Printf("WARNING: merge script failed (%v), falling back to static config", err)
				os.Remove(tmpPath)
			} else {
				configPath = tmpPath
				t.Cleanup(func() { os.Remove(tmpPath) })
			}
		}
	} else {
		log.Printf("WARNING: could not find repo root (%v), using static config", err)
	}

	common.CopyFile(configPath, common.ConfigOutputPath)
	err := common.StartAgent(common.ConfigOutputPath, false, false)
	require.NoError(t, err, "failed to start agent")

	t.Cleanup(func() {
		common.StopAgent()
		common.DeleteFile(common.ConfigOutputPath)
	})

	time.Sleep(2 * time.Minute)

	for category, metrics := range metricCategories {
		category, metrics := category, metrics
		t.Run(category, func(t *testing.T) {
			t.Parallel()
			verifyMetricsPublished(t, metrics)
		})
	}
}

func verifyMetricsPublished(t *testing.T, expectedMetrics []string) {
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Minute)

	for time.Now().Before(deadline) {
		allFound := true
		for _, metricName := range expectedMetrics {
			out, err := awsservice.CwmClient.ListMetrics(ctx, &cloudwatch.ListMetricsInput{
				Namespace:  aws.String(namespace),
				MetricName: aws.String(metricName),
			})
			if err != nil || len(out.Metrics) == 0 {
				allFound = false
				break
			}
		}
		if allFound {
			for _, metricName := range expectedMetrics {
				assert.True(t, true, "metric %s found", metricName)
			}
			return
		}
		time.Sleep(30 * time.Second)
	}

	// Final check with assertions
	listAllMetrics(t)
	for _, metricName := range expectedMetrics {
		out, err := awsservice.CwmClient.ListMetrics(ctx, &cloudwatch.ListMetricsInput{
			Namespace:  aws.String(namespace),
			MetricName: aws.String(metricName),
		})
		assert.NoError(t, err, "error listing metric %s", metricName)
		assert.NotEmpty(t, out.Metrics, "metric %s not found in namespace %s", metricName, namespace)
	}
}

func listAllMetrics(t *testing.T) {
	ctx := context.Background()
	var nextToken *string
	var allMetrics []types.Metric

	for {
		out, err := awsservice.CwmClient.ListMetrics(ctx, &cloudwatch.ListMetricsInput{
			Namespace: aws.String(namespace),
			NextToken: nextToken,
		})
		if err != nil {
			log.Printf("error listing all metrics in namespace %s: %v", namespace, err)
			return
		}
		allMetrics = append(allMetrics, out.Metrics...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	log.Printf("=== All metrics in namespace %s (%d total) ===", namespace, len(allMetrics))
	for _, m := range allMetrics {
		log.Printf("  %s", aws.ToString(m.MetricName))
	}
}
