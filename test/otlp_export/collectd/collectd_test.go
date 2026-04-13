// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package collectd_otlp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

type CollectdOtlpTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (s *CollectdOtlpTestSuite) SetupSuite() {
	log.Println(">>>> Starting CollectdOtlpTestSuite")
}

func (s *CollectdOtlpTestSuite) TearDownSuite() {
	s.Result.Print()
	log.Println(">>>> Finished CollectdOtlpTestSuite")
}

func (s *CollectdOtlpTestSuite) TestAllInSuite() {
	runner := &CollectdOtlpTestRunner{}
	s.AddToSuiteResult(runner.run())
	s.Assert().Equal(status.SUCCESSFUL, s.Result.GetStatus(), "CollectdOtlp Test Suite Failed")
}

func TestCollectdOtlpSuite(t *testing.T) {
	suite.Run(t, new(CollectdOtlpTestSuite))
}

type CollectdOtlpTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectdOtlpTestRunner)(nil)

const yamlConfigPath = "/tmp/config.yaml"
const yamlStartCommand = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -s -c "

func (t *CollectdOtlpTestRunner) run() status.TestGroupResult {
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
	for i := 0; i < 15; i++ {
		body := bytes.NewBufferString("[]")
		resp, err := http.Post("http://127.0.0.1:25826/", "application/json", body)
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err := t.SetupAfterAgentRun(); err != nil {
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{{Name: "Setup After Agent Run", Status: status.FAILED, Reason: err}},
		}
	}
	time.Sleep(30 * time.Second)
	common.StopAgent()
	return t.Validate()
}

func (t *CollectdOtlpTestRunner) GetTestName() string            { return "CollectdOtlp" }
func (t *CollectdOtlpTestRunner) GetAgentConfigFileName() string { return "collectd_otlp.yaml" }
func (t *CollectdOtlpTestRunner) GetAgentRunDuration() time.Duration {
	return 4 * time.Minute
}
func (t *CollectdOtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{"gauge.gauge_1", "counter.counter_1"}
}

func (t *CollectdOtlpTestRunner) SetupAfterAgentRun() error {
	return sendCollectdHTTPMetrics(t.GetAgentRunDuration())
}

func (t *CollectdOtlpTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), "us-west-2", t.GetMeasuredMetrics())
}

func sendCollectdHTTPMetrics(duration time.Duration) error {
	metrics := []map[string]interface{}{
		{"values": []int{1}, "dstypes": []string{"gauge"}, "dsnames": []string{"value"}, "time": 0, "interval": 10, "host": "testhost", "plugin": "gauge_1", "plugin_instance": "", "type": "gauge", "type_instance": "gauge_1"},
		{"values": []int{1}, "dstypes": []string{"counter"}, "dsnames": []string{"value"}, "time": 0, "interval": 10, "host": "testhost", "plugin": "counter_1", "plugin_instance": "", "type": "counter", "type_instance": "counter_1"},
	}
	end := time.Now().Add(duration)
	for time.Now().Before(end) {
		for j := range metrics {
			metrics[j]["time"] = time.Now().Unix()
		}
		body, err := json.Marshal(metrics)
		if err != nil {
			return fmt.Errorf("marshal error: %w", err)
		}
		resp, err := http.Post("http://127.0.0.1:25826/", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("collectd post error: %v", err)
		} else {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
	return nil
}
