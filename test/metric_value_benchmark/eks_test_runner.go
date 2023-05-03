package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
	"time"
)

type EKSTestRunner struct {
	runner test_runner.ITestRunner
	env    environment.MetaData
}

func (t *EKSTestRunner) Run(s test_runner.ITestSuite, e *environment.MetaData) {
	name := t.runner.GetTestName()
	log.Printf("Running %s", name)
	dur := t.runner.GetAgentRunDuration()
	time.Sleep(dur)

	res := t.runner.Validate()
	s.AddToSuiteResult(res)
	if res.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}

type EKSDaemonTestRunner struct {
	test_runner.BaseTestRunner
}

func (e *EKSDaemonTestRunner) Validate() status.TestGroupResult {
	res := status.TestGroupResult{
		Name:        e.GetTestName(),
		TestResults: []status.TestResult{},
	}

	return res
}

func (e *EKSDaemonTestRunner) GetTestName() string {
	return "EKSDaemon" // TODO: what value should go here?
}

func (e *EKSDaemonTestRunner) GetAgentConfigFileName() string {
	return "" // TODO: maybe not needed?
}

func (e *EKSDaemonTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (e *EKSDaemonTestRunner) GetMeasuredMetrics() []string {
	return []string{}
}

func (e *EKSDaemonTestRunner) SetupBeforeAgentRun() error {
	return nil
}

func (e *EKSDaemonTestRunner) SetupAfterAgentRun() error {
	return nil
}

var _ test_runner.ITestRunner = (*EKSDaemonTestRunner)(nil)
