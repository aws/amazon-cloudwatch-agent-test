// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"bufio"
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

var metadata *environment.MetaData

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type AssumeRoleTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *AssumeRoleTestSuite) SetupSuite() {
	log.Println(">>>> Starting AssumeRoleTestSuite")
}

func (suite *AssumeRoleTestSuite) TearDownSuite() {
	suite.Result.Print()
	log.Println(">>>> Finished AssumeRoleTestSuite")
}

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_arn_key",
					name:           "SourceArnKeyOnlyTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   false,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_account_key",
					name:           "SourceAccountKeyOnlyTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   false,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "AllKeysTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   false,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_arn_key",
					name:           "MissingSourceArnEnvTest",
				},
				setSourceArnEnvVar:        false,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-source_account_key",
					name:           "MissingSourceAccountEnvTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    false,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "ContextKeyMismatchAccountTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     false,
				useIncorrectSourceAccount: true,
				expectAssumeRoleFailure:   true,
			},
		},
		{
			TestRunner: &ConfusedDeputyAssumeRoleTestRunner{
				AssumeRoleTestRunner: AssumeRoleTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{},
					roleSuffix:     "-all_context_keys",
					name:           "ContextKeyMismatchArnTest",
				},
				setSourceArnEnvVar:        true,
				setSourceAccountEnvVar:    true,
				useIncorrectSourceArn:     true,
				useIncorrectSourceAccount: false,
				expectAssumeRoleFailure:   true,
			},
		},
	}
)

func (suite *AssumeRoleTestSuite) TestAllInSuite() {
	metadata = environment.GetEnvironmentMetaData()

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
	return metadata.AssumeRoleArn + t.roleSuffix
}

func (t *AssumeRoleTestRunner) setupAgentConfig() error {

	log.Printf("Role ARN: %s\n", t.getRoleArn())
	log.Printf("Metric namespace: %s\n", t.GetTestName())

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
	factory := dimension.GetDimensionFactory(*metadata)
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

	setSourceAccountEnvVar    bool
	useIncorrectSourceAccount bool

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

// validateNoMetrics checks that there were no metrics emitted related to the specific test run
func (t *ConfusedDeputyAssumeRoleTestRunner) validateNoMetrics() []status.TestResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetricMissing(metricName)
	}
	return testResults
}

// validateNoMetrics checks that there were no metric data points for a specific metric related to a specific test run
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
		log.Printf("Unable to fetch metrics: %s", err)
		return testResult
	}

	// fetcher should return no data as the agent should not be able to assume the role it was given
	// If there are values, then something went wrong
	if len(values) > 0 {
		log.Printf("Found %d data values when none were expected\n", len(values))
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

// validateAccessDenied checks that the agent's STS Assume Role call failed using the agent logs
func (t *ConfusedDeputyAssumeRoleTestRunner) validateAccessDenied() status.TestResult {

	testResult := status.TestResult{
		Name:   "access_denied",
		Status: status.FAILED,
	}

	content, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		log.Printf("Unable to open agent log file: %s\n", err)
		return testResult
	}

	// Check for accsess denied error in the agent log
	//
	// Example log
	// ---[ RESPONSE ]--------------------------------------
	// HTTP/1.1 403 Forbidden
	// Content-Length: 444
	// Content-Type: text/xml
	// Date: Wed, 20 Nov 2024 22:56:17 GMT
	// X-Amzn-Requestid: <snip>
	//
	//
	// -----------------------------------------------------
	// 2024-11-20T22:56:17Z I! <ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
	// 	<Error>
	// 		<Type>Sender</Type>
	// 		<Code>AccessDenied</Code>
	// 		<Message>User: arn:aws:sts::<snip>:assumed-role/<role name>/<instance arn> is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam::<snip>:role/CloudWatchLogsPusher</Message>
	// 	</Error>
	// 	<RequestId><snip></RequestId>
	// </ErrorResponse>
	if strings.Contains(string(content), fmt.Sprintf("not authorized to perform: sts:AssumeRole on resource: %s", t.getRoleArn())) {
		log.Println("Found 'not authorized to perform...' in the file")
		testResult.Status = status.SUCCESSFUL
	} else {
		log.Println("Did not find 'not authorized to perform...' in the file")
		testResult.Status = status.FAILED
	}

	return testResult
}

// validateFoundConfusedDeputyHeaders checks that the agent used confued deputy headers in the STS assume role calls
// using the agent's logs
func (t *ConfusedDeputyAssumeRoleTestRunner) validateFoundConfusedDeputyHeaders() status.TestResult {

	testResult := status.TestResult{
		Name:   "confused_deputy_headers",
		Status: status.FAILED,
	}

	file, err := os.Open(common.AgentLogFile)
	if err != nil {
		log.Printf("Error opening agent log file: %v\n", err)
		return testResult
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	inHttpDebug := false
	isStsAssumeRoleRequest := false
	httpDebugLog := []string{}

	// Example HTTP debug log
	//
	// ---[ REQUEST POST-SIGN ]-----------------------------
	// POST / HTTP/1.1
	// Host: sts.us-west-2.amazonaws.com
	// User-Agent: aws-sdk-go/1.48.6 (go1.22.11; linux; arm64)
	// Content-Length: 199
	// Authorization: AWS4-HMAC-SHA256 Credential=<snip>/<snip>/us-west-2/sts/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-security-token;x-amz-source-account;x-amz-source-arn, Signature=<snip>
	// Content-Type: application/x-www-form-urlencoded; charset=utf-8
	// X-Amz-Date: 20250129T170140Z
	// X-Amz-Security-Token: <token>
	// X-Amz-Source-Account: 0123456789012
	// X-Amz-Source-Arn: arn:aws:ec2:us-west-2:123456789012:instance/i-1234567890abcdef0
	// Accept-Encoding: gzip
	//
	// Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A506463145083%3Arole%2Fcwa-integ-assume-role-5be6d1574e9843bb-all_context_keys&RoleSessionName=1738170071781577224&Version=2011-06-15
	// -----------------------------------------------------
	for scanner.Scan() {
		line := scanner.Text()

		// Look for the start of an HTTP request debug log
		if strings.Contains(line, "---[ REQUEST POST-SIGN ]-----------------------------") {
			inHttpDebug = true
			httpDebugLog = []string{}
			isStsAssumeRoleRequest = false
			continue
		}

		// Ignore anything thats not part of an HTTP request debug log
		if !inHttpDebug {
			continue
		}

		httpDebugLog = append(httpDebugLog, line)

		if strings.Contains(line, "Action=AssumeRole") {
			isStsAssumeRoleRequest = true
		}

		// Look for the end of an HTTP request debug log
		if strings.Contains(line, "-----------------------------------------------------") {

			if isStsAssumeRoleRequest && checkForConfusedDeputyHeaders(httpDebugLog) {
				log.Println("Found confused deputy headers in the HTTP debug log")
				testResult.Status = status.SUCCESSFUL
			}

			// Reset the search
			inHttpDebug = false
			isStsAssumeRoleRequest = false
			httpDebugLog = []string{}
		}

	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading file: %v\n", err)
	}

	return testResult
}

// checkForConfusedDeputyHeaders checks for the presence of the confused deputy headers in the HTTP debug log
func checkForConfusedDeputyHeaders(httpDebugLog []string) bool {
	foundSourceAccount := false
	foundSourceArn := false
	for _, line := range httpDebugLog {
		if strings.Contains(line, fmt.Sprintf("X-Amz-Source-Account: %s", metadata.AccountId)) {
			log.Println("Found X-Amz-Source-Account in the HTTP Debug Log")
			foundSourceAccount = true
		}
		if strings.Contains(line, fmt.Sprintf("X-Amz-Source-Arn: %s", metadata.InstanceArn)) {
			log.Println("Found X-Amz-Source-Arn in the HTTP Debug Log")
			foundSourceArn = true
		}
	}

	return foundSourceAccount && foundSourceArn
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

// setupEnvironmentVariables sets the agent's environment variables using the systemd service file
func (t *ConfusedDeputyAssumeRoleTestRunner) setupEnvironmentVariables() error {

	// Set or remove the environment variables in the service file
	common.CopyFile("service_configs/amazon-cloudwatch-agent.service", "/etc/systemd/system/amazon-cloudwatch-agent.service")

	if t.setSourceAccountEnvVar {
		sourceAccount := metadata.AccountId
		if t.useIncorrectSourceAccount {
			sourceAccount = "123456789012"
		}

		log.Printf("AMZ_SOURCE_ACCOUNT: %s\n", sourceAccount)

		sedCmd := fmt.Sprintf("sudo sed -i 's|ACCOUNT_PLACEHOLDER|%s|g' /etc/systemd/system/amazon-cloudwatch-agent.service", sourceAccount)
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to replace AMZ_SOURCE_ACCOUNT value: %w", err)
		}
	} else {
		log.Println("Removing AMZ_SOURCE_ACCOUNT from service file")

		sedCmd := "sudo sed -i '/AMZ_SOURCE_ACCOUNT/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed remove PLACEHOLDER value: %w", err)
		}
	}

	if t.setSourceArnEnvVar {
		sourceArn := metadata.InstanceArn
		if t.useIncorrectSourceArn {
			sourceArn = "arn:aws:ec2:us-west-2:123456789012:instance/i-1234567890abcdef0"
		}

		log.Printf("AMZ_SOURCE_ARN: %s\n", sourceArn)

		sedCmd := fmt.Sprintf("sudo sed -i 's|ARN_PLACEHOLDER|%s|g' /etc/systemd/system/amazon-cloudwatch-agent.service", sourceArn)
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to replace AMZ_SOURCE_ARN value: %w", err)
		}

	} else {
		log.Println("Removing AMZ_SOURCE_ARN from service file")

		sedCmd := "sudo sed -i '/AMZ_SOURCE_ARN/d' /etc/systemd/system/amazon-cloudwatch-agent.service"
		cmd := exec.Command("bash", "-c", sedCmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to remove AMZ_SOURCE_ARN value: %w", err)
		}
	}

	err := t.daemonReload()
	if err != nil {
		return err
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
