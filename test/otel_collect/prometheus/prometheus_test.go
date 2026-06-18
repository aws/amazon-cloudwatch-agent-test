// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package prometheus

import (
	_ "embed"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

//go:embed resources/prometheus_scrape_config.yaml
var prometheusScrapeConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

const prometheusRuntime = 3 * time.Minute

type PrometheusOtelTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*PrometheusOtelTestRunner)(nil)

func (t *PrometheusOtelTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics(), map[string]string{
		"@resource.host.id": t.env.InstanceId,
	})
}

func (t *PrometheusOtelTestRunner) GetTestName() string                { return "OtelCollectPrometheus" }
func (t *PrometheusOtelTestRunner) GetAgentRunDuration() time.Duration { return prometheusRuntime }
func (t *PrometheusOtelTestRunner) GetAgentConfigFileName() string     { return "prometheus_config.json" }
func (t *PrometheusOtelTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"node_cpu_seconds_total",
		"node_memory_MemAvailable_bytes",
		"node_filesystem_avail_bytes",
		"node_network_receive_bytes_total",
	}
}

func (t *PrometheusOtelTestRunner) SetupBeforeAgentRun() error {
	if err := t.BaseTestRunner.SetupBeforeAgentRun(); err != nil {
		return err
	}

	// Write prometheus scrape config
	commands := []string{
		fmt.Sprintf("cat <<'EOF' | sudo tee /opt/aws/prometheus.yml\n%s\nEOF", prometheusScrapeConfig),
	}
	if err := common.RunCommands(commands); err != nil {
		return err
	}

	// Serve fake metrics on port 9100 (same pattern as test/emf_prometheus)
	if err := os.WriteFile("/tmp/metrics", []byte(prometheusMetrics), os.ModePerm); err != nil {
		return fmt.Errorf("unable to write /tmp/metrics: %w", err)
	}
	commands = []string{
		"sudo python3 -m http.server 9100 --directory /tmp &> /dev/null &",
	}
	if err := common.RunCommands(commands); err != nil {
		return err
	}
	t.RegisterCleanup(func() error {
		return common.RunCommands([]string{"sudo pkill -f 'python3 -m http.server 9100' || true"})
	})
	time.Sleep(2 * time.Second)
	return nil
}

func TestPrometheus(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &PrometheusOtelTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
