package testrunners

import (
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/rule"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
	"strings"
	"time"
)

type FilePermissionTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*FilePermissionTestRunner)(nil)

const agentConfigPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
const agentConfigOnlyRootRead = "-rw-r--r-- 1 root root"

var (
	onlyRootReadExactMatchRule = rule.Rule{
		Conditions: []rule.ICondition{
			&rule.ExactMatch{ExpectedValue: agentConfigOnlyRootRead},
		},
	}
)

var testCases = map[string]rule.Rule{
	agentConfigPath: onlyRootReadExactMatchRule,
}

var actualPermissions = make(map[string]string)

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
	for k, _ := range testCases {
		p, err := getFilePermission(k)
		if err != nil {
			return err
		}

		log.Printf("file perission is %s", p)
		actualPermissions[k] = p
	}

	return nil
}

func (m *FilePermissionTestRunner) validatePermissions(fileTestedPath string, rule rule.Rule) status.TestResult {
	log.Printf("Validating Permission for  %v", fileTestedPath)

	testResult := status.TestResult{
		Name:   fileTestedPath,
		Status: status.FAILED,
	}

	if !rule.Evaluate(actualPermissions[fileTestedPath]) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func getFilePermission(filePath string) (string, error) {
	log.Printf("Retrieving file permission for %v", filePath)
	p, err := common.RunCommand("ls -l " + agentConfigPath)
	if err != nil {
		return "", err
	}

	slice := strings.SplitAfterN(p, " ", 5)
	onlyFirst4WordsInPermission := strings.Join(slice[:4], "")

	return onlyFirst4WordsInPermission, nil
}
