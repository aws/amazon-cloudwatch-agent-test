// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type AssumeRoleTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *AssumeRoleTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting AssumeRoleTestSuite")
}

func (suite *AssumeRoleTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished AssumeRoleTestSuite")
}

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &AssumeRoleTestRunner{
				BaseTestRunner: test_runner.BaseTestRunner{},
				name:           "AssumeRoleTest",
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_arn_key",
					name:           "SourceArnKeyOnlyTest",
				},
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_account_key",
					name:           "SourceAccountKeyOnlyTest",
				},
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "AllKeysTest",
				},
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_arn_key",
					name:           "MissingSourceArnEnvTest",
				},
				setSourceArnEnvVar:      true,
				expectAssumeRoleFailure: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_account_key",
					name:           "MissingSourceAccountEnvTest",
				},
				setSourceAccountEnvVar:  true,
				expectAssumeRoleFailure: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "ContextKeyMismatchAccountTest",
				},
				setSourceArnEnvVar:       true,
				setSourceAccountEnvVar:   true,
				useInorrectSourceAccount: true,
				expectAssumeRoleFailure:  true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "ContextKeyMismatchArnTest",
				},
				setSourceArnEnvVar:      true,
				useIncorrectSourceArn:   true,
				setSourceAccountEnvVar:  true,
				expectAssumeRoleFailure: true,
			},
		},
	}
)

func (suite *AssumeRoleTestSuite) TestAllInSuite() {
	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Assume Role Test Suite Failed")
}

type AssumeRoleTestRunner struct {
	test_runner.BaseTestRunner

	name string

	// terraform will create several roles which all share a base name and have a unique prefix. the base ARN is passed
	// in via command line parameter, and the other roles can be referenced by appending a suffix to the base ARN
	roleSuffix string
}

func (t AssumeRoleTestRunner) Validate() status.TestGroupResult {
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: t.validateMetrics(),
	}
}

func (t AssumeRoleTestRunner) validateMetrics() []status.TestResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName)
	}
	return testResults
}

func (t *AssumeRoleTestRunner) validateMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims := getDimensions()
	if len(dims) == 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(t.GetTestName(), metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t AssumeRoleTestRunner) GetTestName() string {
	return t.name
}

func (t AssumeRoleTestRunner) GetAgentConfigFileName() string {
	return "agent_configs/config.json"
}

func (t AssumeRoleTestRunner) GetMeasuredMetrics() []string {
	return metric.CpuMetrics
}

func (t *AssumeRoleTestRunner) SetupBeforeAgentRun() error {
	return t.setupAgentConfig()
}

func (t *AssumeRoleTestRunner) getRoleArn() string {
	// Role ARN used by these tests assume a basic role name (given by the AssumeRoleArn environment metadata) with
	// and optional suffix
	return environment.GetEnvironmentMetaData().AssumeRoleArn + t.roleSuffix
}

func (t *AssumeRoleTestRunner) setupAgentConfig() error {

	fmt.Printf("Role ARN: %s\n", t.getRoleArn())
	fmt.Printf("Metric namespace: %s\n", t.GetTestName())

	// The default agent config file conatins a ROLE_ARN_PLACEHOLDER value which should be replaced with the ARN of the role
	// that the agent should assume. The ARN is not known until runtime. Test runner does not have sudo permissions,
	// but it can execute sudo commands. Use sed to update the ROLE_ARN_PLACEHOLDER value instead of using built-ins
	common.CopyFile(t.AgentConfig.ConfigFileName, configOutputPath)

	sedCmd := fmt.Sprintf("sudo sed -i 's|ROLE_ARN_PLACEHOLDER|%s|g' %s", t.getRoleArn(), configOutputPath)
	cmd := exec.Command("bash", "-c", sedCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed replace ROLE_ARN_PLACEHOLDER value: %w", err)
	}

	sedCmd = fmt.Sprintf("sudo sed -i 's|NAMESPACE_PLACEHOLDER|%s|g' %s", t.GetTestName(), configOutputPath)
	cmd = exec.Command("bash", "-c", sedCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed replace NAMESPACE_PLACEHOLDER value: %w", err)
	}

	return nil
}

var _ test_runner.ITestRunner = (*AssumeRoleTestRunner)(nil)

func getDimensions() []types.Dimension {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}

type ConfusedDeputyAssumeRoleTestRunner struct {
	AssumeRoleTestRunner

	setSourceArnEnvVar    bool
	useIncorrectSourceArn bool

	setSourceAccountEnvVar   bool
	useInorrectSourceAccount bool

	expectAssumeRoleFailure bool
}

func (t *ConfusedDeputyAssumeRoleTestRunner) GetTestName() string {
	return t.name
}

func (t *ConfusedDeputyAssumeRoleTestRunner) Validate() status.TestGroupResult {

	result := status.TestGroupResult{
		Name: t.GetTestName(),
	}

	if t.expectAssumeRoleFailure {
		result.TestResults = append(result.TestResults, t.validateNoMetrics()...)
		result.TestResults = append(result.TestResults, t.validateAccessDenied())
	} else {
		result.TestResults = append(result.TestResults, t.validateMetrics()...)
		result.TestResults = append(result.TestResults, t.validateFoundConfusedDeputyHeaders())
	}

	return result
}

func (t *ConfusedDeputyAssumeRoleTestRunner) validateNoMetrics() []status.TestResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetricMissing(metricName)
	}
	return testResults
}

func (t *AssumeRoleTestRunner) validateMetricMissing(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims := getDimensions()
	if len(dims) == 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(t.GetTestName(), metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	// fetcher should return no data as the agent should not be able to assume the role it was given
	// If there are values, then something went wrong
	if len(values) > 0 {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *ConfusedDeputyAssumeRoleTestRunner) validateAccessDenied() status.TestResult {

	testResult := status.TestResult{
		Name:   "access_denied",
		Status: status.FAILED,
	}

	// Check for accsess denied error in the agent log
	content, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		return testResult
	}

	if strings.Contains(string(content), fmt.Sprintf("not authorized to perform: sts:AssumeRole on resource: %s", t.getRoleArn())) {
		fmt.Println("Found 'not authorized to perform...' in the file")
		testResult.Status = status.SUCCESSFUL
	} else {
		fmt.Println("Did not find 'not authorized to perform...' in the file")
		testResult.Status = status.FAILED
	}

	return testResult
}

func (t *ConfusedDeputyAssumeRoleTestRunner) validateFoundConfusedDeputyHeaders() status.TestResult {
	// To double check that the agent was actually using confused deputy headers in the assume role calls,
	// check for the informational debug output in the log file. This is a bit frivolous since it relies on the
	// logging functionality of the agent, so it could be removed if it causes problems
	testResult := status.TestResult{
		Name:   "confused_deputy_headers",
		Status: status.FAILED,
	}

	content, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		return testResult
	}

	if strings.Contains(string(content), "Found confused deputy header environment variables") {
		fmt.Println("Found 'confused deputy header variables' in the logs")
		testResult.Status = status.SUCCESSFUL
	} else {
		fmt.Println("Did not find 'confused deputy header variables' in the file")
		testResult.Status = status.FAILED
	}

	return testResult
}

func (t *ConfusedDeputyAssumeRoleTestRunner) SetupBeforeAgentRun() error {
	err := t.setupEnvironmentVariables()
	if err != nil {
		return fmt.Errorf("failed to setup environment variables: %w", err)
	}

	// Clear out log file since we'll need to check the logs on each run and we don't want logs from another test
	// being checked
	err = t.clearLogFile()
	if err != nil {
		return fmt.Errorf("failed to clear log file: %w", err)
	}
	return t.setupAgentConfig()
}

func (t *ConfusedDeputyAssumeRoleTestRunner) setupEnvironmentVariables() error {

	// The default service file will set the AMZ_SOURCE_ARN and AMZ_SOURCE_ACCOUNT environment varaibles. If the test
	// calls for one or more of those variables to be unset, those lines will be removed.
	//
	// The AMZ_SOURCE_ARN line also contains a PLACEHOLDER value which should be filled in with the ARN of the instance
	// running this test. The ARN isn't known until runtime. Test runner does not have sudo permissions, but it can
	// execute sudo commands. Use sed to update the PLACEHOLDER value instead of using built-ins

	if t.useIncorrectSourceArn {
		common.CopyFile("service_configs/incorrect_source_account.service", "/etc/systemd/system/amazon-cloudwatch-agent.service")
	} else {
		common.CopyFile("service_configs/amazon-cloudwatch-agent.service", "/etc/systemd/system/amazon-cloudwatch-agent.service")
	}

	if !t.setSourceAccountEnvVar {
		fmt.Println("Removing AMZ_SOURCE_ACCOUNT from service file")

		sedCmd := "sudo sed -i '/AMZ_SOURCE_ACCOUNT/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed remove PLACEHOLDER value: %w", err)
		}

		err := t.daemonReload()
		if err != nil {
			return err
		}
	}

	if !t.setSourceArnEnvVar {
		fmt.Println("Removing AMZ_SOURCE_ARN from service file")

		sedCmd := "sudo sed -i '/AMZ_SOURCE_ARN/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to remove AMZ_SOURCE_ARN value: %w", err)
		}

		err := t.daemonReload()
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("AMZ_SOURCE_ARN: %s\n", environment.GetEnvironmentMetaData().InstanceArn)

		sedCmd := fmt.Sprintf("sudo sed -i 's|PLACEHOLDER|%s|g' /etc/systemd/system/amazon-cloudwatch-agent.service", environment.GetEnvironmentMetaData().InstanceArn)
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to replace AMZ_SOURCE_ARN value: %w", err)
		}

		err := t.daemonReload()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *ConfusedDeputyAssumeRoleTestRunner) daemonReload() error {
	cmd := exec.Command("sudo", "systemctl", "daemon-reload")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to daemon-reload: %w; command output: %s", err, string(output))
	}
	return nil
}

func (t *ConfusedDeputyAssumeRoleTestRunner) clearLogFile() error {
	cmd := exec.Command("sudo", "rm", common.AgentLogFile)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to clear log file: %w; command output: %s", err, string(output))
	}
	return nil
}
