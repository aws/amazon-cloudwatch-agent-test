// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd_otlp

import (
	"log"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otlp_export/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type StatsdOtlpTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (s *StatsdOtlpTestSuite) SetupSuite() {
	log.Println(">>>> Starting StatsdOtlpTestSuite")
}

func (s *StatsdOtlpTestSuite) TearDownSuite() {
	s.Result.Print()
	log.Println(">>>> Finished StatsdOtlpTestSuite")
}

func (s *StatsdOtlpTestSuite) TestAllInSuite() {
	runner := &StatsdOtlpTestRunner{done: make(chan bool)}
	s.AddToSuiteResult(runner.run())
	s.Assert().Equal(status.SUCCESSFUL, s.Result.GetStatus(), "StatsdOtlp Test Suite Failed")
}

func TestStatsdOtlpSuite(t *testing.T) {
	suite.Run(t, new(StatsdOtlpTestSuite))
}

type StatsdOtlpTestRunner struct {
	test_runner.BaseTestRunner
	done chan bool
}

var _ test_runner.ITestRunner = (*StatsdOtlpTestRunner)(nil)

const yamlConfigPath = "/tmp/config.yaml"
const yamlStartCommand = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -s -c "

func (t *StatsdOtlpTestRunner) run() status.TestGroupResult {
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
	if err := t.SetupAfterAgentRun(); err != nil {
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{{Name: "Setup After Agent Run", Status: status.FAILED, Reason: err}},
		}
	}
	time.Sleep(t.GetAgentRunDuration())
	close(t.done)
	common.StopAgent()
	return t.Validate()
}

func (t *StatsdOtlpTestRunner) GetTestName() string            { return "StatsdOtlp" }
func (t *StatsdOtlpTestRunner) GetAgentConfigFileName() string { return "statsd_otlp.yaml" }
func (t *StatsdOtlpTestRunner) GetAgentRunDuration() time.Duration {
	return 4 * time.Minute
}
func (t *StatsdOtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{"statsd_counter_1", "statsd_gauge_2"}
}

func (t *StatsdOtlpTestRunner) SetupAfterAgentRun() error {
	go metric.SendStatsdMetricsWithEntity(t.done)
	return nil
}

func (t *StatsdOtlpTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), "us-west-2", t.GetMeasuredMetrics())
}
