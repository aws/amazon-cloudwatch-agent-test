package testrunners

import (
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/rule"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
	"time"
)

type FilePermissionTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*FilePermissionTestRunner)(nil)

const agentConfigPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"

var (
	onlyRootCanWriteRule = rule.Rule[string]{
		Conditions: []rule.ICondition[string]{
			&rule.PermittedEntityMatch{ExpectedOwner: aws.String("root"), ExpectedGroup: aws.String("root")},
			&rule.FilePermissionExpected{PermissionCompared: filesystem.OwnerWrite, ShouldExist: true},
			&rule.FilePermissionExpected{PermissionCompared: filesystem.GroupWrite, ShouldExist: true},
			&rule.FilePermissionExpected{PermissionCompared: filesystem.AnyoneWrite, ShouldExist: false},
		},
	}
)

var testCases = map[string]rule.Rule[string]{
	agentConfigPath: onlyRootCanWriteRule,
}

var testGroupResult *status.TestGroupResult = nil

func (m *FilePermissionTestRunner) Validate() status.TestGroupResult {
	if testGroupResult == nil {
		return m.createTestGroupFailure()
	} else {
		return *testGroupResult
	}
}

func (m *FilePermissionTestRunner) GetTestName() string {
	return "FilePermission"
}

func (m *FilePermissionTestRunner) GetAgentConfigFileName() string {
	return "minimum_config.json"
}

func (m *FilePermissionTestRunner) GetMeasuredMetrics() []string {
	return []string{}
}

func (m *FilePermissionTestRunner) GetAgentRunDuration() time.Duration {
	return 1 * time.Minute
}

func (m *FilePermissionTestRunner) SetupAfterAgentRun() error {
	testResults := make([]status.TestResult, len(testCases))

	count := 0
	for k, v := range testCases {
		testResults[count] = m.validatePermissions(k, v)
		count++
	}

	testGroupResult = &status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
	return nil
}

func (m *FilePermissionTestRunner) validatePermissions(fileTestedPath string, rule rule.Rule[string]) status.TestResult {
	log.Printf("Validating Permission for  %v", fileTestedPath)

	testResult := status.TestResult{
		Name:   fileTestedPath,
		Status: status.FAILED,
	}

	success, err := rule.Evaluate(fileTestedPath)
	if err != nil || !success {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (m *FilePermissionTestRunner) createTestGroupFailure() status.TestGroupResult {
	testResult := status.TestResult{
		Name:   "",
		Status: status.FAILED,
	}
	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: []status.TestResult{testResult},
	}
}
