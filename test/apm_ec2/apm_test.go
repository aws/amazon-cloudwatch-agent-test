// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package apm_ec2

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const testRetryCount = 3
const namespace = "AWS/APM"

var path, testId string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()

	flag.StringVar(&path, "path", "./resources/run_java_application.sh", "path to java script file.")
	flag.StringVar(&testId, "test.id", "unknown", "testing id.")
}

type APMEC2TestRunner struct {
	test_runner.BaseTestRunner
}

func (runner *APMEC2TestRunner) SetupAfterAgentRun() error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return errors.New(fmt.Sprintf("file %s does not exist.", path))
	}
	if output, err := exec.Command("bash", "-c", "sudo chmod +x "+path).Output(); err != nil {
		return errors.New(fmt.Sprintf("failed to execute chmod: %s.", string(output)))
	}

	cmd := exec.Command("bash", "-c", "./"+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TESTING_ID="+testId)

	if err := cmd.Start(); err != nil {
		return errors.New(fmt.Sprintf("failed to start application: %v.", err))
	}

	defer func() {
		// Using context cancel doesn't kill child process, kill process group instead.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			log.Fatalf("failed to stop application: %v.", err)
		} else {
			log.Printf("stop application.")
		}
		cmd.Wait()
	}()

	success := false
	for i := 1; i < 10; i++ {
		if resp, err := http.Get("http://localhost:8080/api/gateway/owners/1"); err != nil {
			log.Printf("failed to send out request: %v.", err)
			time.Sleep(time.Duration(i*10) * time.Second)
		} else if resp.StatusCode >= 500 {
			log.Printf("returned failure response: %d", resp.StatusCode)
			time.Sleep(10000)
		} else {
			success = true
			log.Printf("successfully sent out the request.")
		}
	}

	if !success {
		return errors.New("failed to call the test service")
	}

	// Sleep 1 minute to let metrics and traces be exported.
	time.Sleep(90 * time.Second)

	return nil
}

func (runner *APMEC2TestRunner) Validate() status.TestGroupResult {
	metricsToFetch := runner.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	instructions := metric.EC2ServerConsumerInstructions

	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		for j := 0; j < testRetryCount; j++ {
			log.Printf("validating metric - name: %s.", metricName)
			testResult = metric.ValidateAPMMetric(runner.DimensionFactory, namespace, metricName, instructions)
			if testResult.Status == status.SUCCESSFUL {
				break
			}
			time.Sleep(15 * time.Second)
		}
		testResults[i] = testResult
	}

	return status.TestGroupResult{
		Name:        runner.GetTestName(),
		TestResults: testResults,
	}
}

func (runner *APMEC2TestRunner) GetTestName() string {
	return "APMTest/EC2"
}

func (runner *APMEC2TestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (runner *APMEC2TestRunner) GetMeasuredMetrics() []string {
	return metric.APMMetricNames
}

func (runner *APMEC2TestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

var _ test_runner.ITestRunner = (*APMEC2TestRunner)(nil)

func TestHostedInAttributes(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	ec2TestRunner := &APMEC2TestRunner{test_runner.BaseTestRunner{
		DimensionFactory: dimension.GetDimensionFactory(*env),
	}}
	testRunner := test_runner.TestRunner{TestRunner: ec2TestRunner}
	result := testRunner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("APM test on EC2 failed.")
		result.Print()
	}
}
