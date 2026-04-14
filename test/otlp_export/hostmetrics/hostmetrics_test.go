// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package hostmetrics

import (
	"log"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otlp_export/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type HostMetricsOtlpTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (s *HostMetricsOtlpTestSuite) SetupSuite() {
	log.Println(">>>> Starting HostMetricsOtlpTestSuite")
}

func (s *HostMetricsOtlpTestSuite) TearDownSuite() {
	s.Result.Print()
	log.Println(">>>> Finished HostMetricsOtlpTestSuite")
}

func (s *HostMetricsOtlpTestSuite) TestAllInSuite() {
	runner := &HostMetricsOtlpTestRunner{}
	s.AddToSuiteResult(runner.run())
	s.Assert().Equal(status.SUCCESSFUL, s.Result.GetStatus(), "HostMetricsOtlp Test Suite Failed")
}

func TestHostMetricsOtlpSuite(t *testing.T) {
	suite.Run(t, new(HostMetricsOtlpTestSuite))
}

type HostMetricsOtlpTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*HostMetricsOtlpTestRunner)(nil)

const yamlConfigPath = "/tmp/config.yaml"
const yamlStartCommand = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -s -c "

func (t *HostMetricsOtlpTestRunner) run() status.TestGroupResult {
	if err := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "remove-config").Run(); err != nil {
		log.Printf("remove-config failed: %v", err)
	}
	common.CopyFile(filepath.Join("agent_configs", t.GetAgentConfigFileName()), yamlConfigPath)
	if err := common.StartAgentWithCommand(yamlConfigPath, false, false, yamlStartCommand); err != nil {
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{{Name: "Starting Agent", Status: status.FAILED, Reason: err}},
		}
	}
	time.Sleep(t.GetAgentRunDuration())
	common.StopAgent()
	result := t.Validate()
	return result
}

func (t *HostMetricsOtlpTestRunner) GetTestName() string            { return "HostMetricsOtlp" }
func (t *HostMetricsOtlpTestRunner) GetAgentConfigFileName() string { return "hostmetrics_otlp.yaml" }
func (t *HostMetricsOtlpTestRunner) GetAgentRunDuration() time.Duration {
	return 4 * time.Minute
}
func (t *HostMetricsOtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{
		// cpu scraper
		"system.cpu.time",
		"system.cpu.utilization",
		// memory scraper
		"system.memory.usage",
		// disk scraper
		"system.disk.io",
		"system.disk.io_time",
		"system.disk.merged",
		"system.disk.operation_time",
		"system.disk.operations",
		"system.disk.pending_operations",
		"system.disk.weighted_io_time",
		// load scraper
		"system.cpu.load_average.1m",
		"system.cpu.load_average.5m",
		"system.cpu.load_average.15m",
		// filesystem scraper
		"system.filesystem.usage",
		"system.filesystem.inodes.usage",
		// network scraper
		"system.network.io",
		"system.network.packets",
		"system.network.dropped",
		"system.network.errors",
		"system.network.connections",
		// paging scraper
		"system.paging.operations",
		"system.paging.faults",
		// processes scraper
		"system.processes.count",
		"system.processes.created",
		// process scraper
		"process.cpu.time",
		"process.memory.usage",
		"process.memory.virtual",
		"process.disk.io",
		// system scraper
		"system.uptime",
	}
}

func (t *HostMetricsOtlpTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), "us-west-2", t.GetMeasuredMetrics())
}
