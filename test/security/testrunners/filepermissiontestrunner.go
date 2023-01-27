package testrunners

import (
	"github.com/aws/amazon-cloudwatch-agent-test/internal/rule"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
)

type FilePermissionTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*FilePermissionTestRunner)(nil)

const agentConfigPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"

var testCases = map[string]rule.Rule{
	agentConfigPath: onlyRootReadExactMatchRule,
}

const agentConfigOnlyRootRead = "-rw-rw-r--"

var onlyRootReadExactMatchRule = rule.Rule{
	Conditions: []*rule.ICondition{
		&rule.ExactMatch{ExpectedValue: agentConfigOnlyRootRead},
	},
}

func (m *FilePermissionTestRunner) Validate() status.TestGroupResult {
	testResults := make([]status.TestResult, len(testCases))

	count := 0
	for k, v := range testCases {
		testResults[count] = m.validatePermissions(k, v)
		count++
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *FilePermissionTestRunner) GetTestName() string {
	return "Processes"
}

func (m *FilePermissionTestRunner) GetAgentConfigFileName() string {
	return "processes_config.json"
}

func (m *FilePermissionTestRunner) GetMeasuredMetrics() []string {
	return []string{}
}

func (m *FilePermissionTestRunner) validatePermissions(fileTestedPath string, rule rule.Rule) status.TestResult {
	log.Printf("Validating Permission for  %v", fileTestedPath)

	testResult := status.TestResult{
		Name:   fileTestedPath,
		Status: status.FAILED,
	}

	// TODO Get permission from Command
	p := getFilePermission(fileTestedPath)

	if !rule.Evaluate(p) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func getFilePermission(filePath string) string {
	log.Printf("Retrieving file permission for %v", filePath)
	return "DummyPermission"
}
