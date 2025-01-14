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
	namespace        = "AssumeRoleTest" // should match whats in agent config file
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
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                   "confused deputy env w/ sourceArn role",
				roleSuffix:             "-source_arn_key",
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                   "confused deputy env w/ sourceAccount role",
				roleSuffix:             "-source_account_key",
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                   "confused deputy env w/ all keys role",
				roleSuffix:             "-all_context_keys",
				setSourceArnEnvVar:     true,
				setSourceAccountEnvVar: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                    "confused deputy env, only source arn",
				roleSuffix:              "-source_arn_key",
				setSourceArnEnvVar:      true,
				expectAssumeRoleFailure: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                    "confused deputy env, only source account",
				roleSuffix:              "-source_account_key",
				setSourceAccountEnvVar:  true,
				expectAssumeRoleFailure: true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
				},
				name:                     "confused deputy env, mismatch account",
				roleSuffix:               "-all_context_keys",
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
				},
				name:                    "confused deputy env, mismatch arn",
				roleSuffix:              "-all_context_keys",
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
}

func (t AssumeRoleTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AssumeRoleTestRunner) validateMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims := getDimensions(environment.GetEnvironmentMetaData().InstanceId)
	if len(dims) == 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

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
	return namespace
}

func (t AssumeRoleTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t AssumeRoleTestRunner) GetMeasuredMetrics() []string {
	return metric.CpuMetrics
}

func (t *AssumeRoleTestRunner) SetupBeforeAgentRun() error {
	err := t.setupAgentConfig()
	if err != nil {
		return fmt.Errorf("failed to setup agent config: %w", err)
	}
	return t.SetUpConfig()
}

func (t *AssumeRoleTestRunner) setupAgentConfig() error {
	// The default agent config file conatins a PLACEHOLDER value which should be replaced with the ARN of the role
	// that the agent should assume. The ARN is not known until runtime. Test runner does not have sudo permissions,
	// but it can execute sudo commands. Use sed to update the PLACEHOLDER value instead of using built-ins
	common.CopyFile("agent_configs/config.json", configOutputPath)
	fmt.Printf("Replacing PLACEHOLDER with %s in %s\n", environment.GetEnvironmentMetaData().InstanceArn, configOutputPath)
	sedCmd := fmt.Sprintf("sudo sed -i 's/PLACEHOLDER/%s/g' %s", environment.GetEnvironmentMetaData().InstanceArn, configOutputPath)
	fmt.Printf("sed command: %s\n", sedCmd)
	cmd := exec.Command("bash", "-c", sedCmd)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed replace PLACEHOLDER value: %s; command output: %s", err, string(output))
	}

	return nil
}

var _ test_runner.ITestRunner = (*AssumeRoleTestRunner)(nil)

func getDimensions(_ string) []types.Dimension {
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

func Validate(assumeRoleArn string) error {
	return nil
}

type ConfusedDeputyAssumeRoleTestRunner struct {
	AssumeRoleTestRunner

	name                     string
	roleSuffix               string
	setSourceArnEnvVar       bool
	useIncorrectSourceArn    bool
	setSourceAccountEnvVar   bool
	useInorrectSourceAccount bool
	expectAssumeRoleFailure  bool
}

func (t *ConfusedDeputyAssumeRoleTestRunner) Validate() status.TestGroupResult {

	if t.expectAssumeRoleFailure {
		return t.validateAccessDenied()
	}

	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *ConfusedDeputyAssumeRoleTestRunner) validateAccessDenied() status.TestGroupResult {
	// Check for accsess denied error in the agent log
	content, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{},
		}
	}
	testResult := status.TestResult{
		Name:   "accessDenied",
		Status: status.FAILED,
	}

	// FOR DEBUGGING ONLY
	fmt.Println("validateAccessDenied agent log content")
	fmt.Println(string(content))

	if strings.Contains(string(content), "AccessDenied") {
		fmt.Println("Found 'AccessDenied' in the file")
		testResult.Status = status.SUCCESSFUL
	}

	return status.TestGroupResult{
		Name: t.GetTestName(),
		TestResults: []status.TestResult{
			testResult,
		},
	}
}

func (t *ConfusedDeputyAssumeRoleTestRunner) SetupBeforeAgentRun() error {
	err := t.setupAgentConfig()
	if err != nil {
		return fmt.Errorf("failed to setup agent config: %w", err)
	}

	err = t.setupEnvironmentVariables()
	if err != nil {
		return fmt.Errorf("failed to setup environment variables: %w", err)
	}

	return t.SetUpConfig()
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
		// Remove the line with AMZ_SOURCE_ACCOUNT
		sedCmd := "sed -i '/AMZ_SOURCE_ACCOUNT/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		fmt.Printf("sed command: %s\n", sedCmd)
		cmd := exec.Command("bash", "-c", sedCmd)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed replace PLACEHOLDER value: %w; command output: %s", err, string(output))
		}
	}

	if !t.setSourceArnEnvVar {
		// Remove the line with AMZ_SOURCE_ARN
		sedCmd := "sed -i '/AMZ_SOURCE_ARN/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		fmt.Printf("sed command: %s\n", sedCmd)
		cmd := exec.Command("bash", "-c", sedCmd)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to replace AMZ_SOURCE_ARN value: %w; command output: %s", err, string(output))
		}
	} else {
		// Replace PLACEHOLDER value in the AMZ_SOURCE_ARN line
		sedCmd := fmt.Sprintf("sudo sed -i 's/PLACEHOLDER/%s/g' /etc/systemd/system/amazon-cloudwatch-agent.service", environment.GetEnvironmentMetaData().InstanceArn)
		fmt.Printf("sed command: %s\n", sedCmd)
		cmd := exec.Command("bash", "-c", sedCmd)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to replace AMZ_SOURCE_ARN value: %w; command output: %s", err, string(output))
		}
	}

	return nil
}
