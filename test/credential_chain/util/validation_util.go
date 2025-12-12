// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package util

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	dimensionTestName   = "TestName"
	dimensionInstanceID = "InstanceId"
	dimensionCpu        = "cpu"
)

// CredentialProviderInfo contains information about the credential provider used
type CredentialProviderInfo struct {
	ProviderName string
	AccessKeyID  string
}

// ParseAgentLogsForCredentialProvider extracts credential provider name from logs
func ParseAgentLogsForCredentialProvider(expectedProvider string) (*CredentialProviderInfo, error) {
	file, err := os.Open(common.AgentLogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open agent log: %w", err)
	}
	defer file.Close()

	// Pattern: "Using credential AKIA... from SharedCredentialsProvider"
	pattern := regexp.MustCompile(`Using credential\s+([A-Z0-9]+)\s+from\s+([^:\s]+)`)

	var lastMatch *CredentialProviderInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := pattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			lastMatch = &CredentialProviderInfo{
				ProviderName: matches[2],
				AccessKeyID:  matches[1],
			}

			if lastMatch.ProviderName == expectedProvider {
				return lastMatch, nil
			}
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading agent log: %w", err)
	}

	if lastMatch != nil {
		return nil, fmt.Errorf("provider mis-match: expected %s, got %s", expectedProvider, lastMatch.ProviderName)
	}

	return nil, fmt.Errorf("no credential provider (%s) not found", expectedProvider)
}

// getDimensions returns the dimensions for metric queries
func getDimensions(testName string, metadata *environment.MetaData) []types.Dimension {
	factory := dimension.GetDimensionFactory(*metadata)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   dimensionTestName,
			Value: dimension.ExpectedDimensionValue{Value: aws.String(testName)},
		},
		{
			Key:   dimensionInstanceID,
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   dimensionCpu,
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}

// ValidateMetric checks if a specific metric was delivered to CloudWatch
func ValidateMetric(testName string, namespace string, metricName string, metadata *environment.MetaData) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("ValidateMetric: %s::%s", namespace, metricName),
		Status: status.FAILED,
	}

	dims := getDimensions(testName, metadata)
	if len(dims) == 0 {
		testResult.Reason = fmt.Errorf("failed to get dimensions")
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	var values []float64
	var err error
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		values, err = fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
		if err == nil && len(values) > 0 {
			log.Printf("Found %d metric values on attempt %d", len(values), attempt+1)
			break
		}

		if attempt < maxRetries-1 {
			waitTime := time.Duration(1<<attempt) * 10 * time.Second // 15s, 30s, 60s, 120s...
			log.Printf("Attempt %d: No metrics found, retrying in %v...", attempt+1, waitTime)
			time.Sleep(waitTime)
		}
	}

	if err != nil {
		testResult.Reason = fmt.Errorf("failed to fetch metric %s: %w", metricName, err)
		return testResult
	}

	if len(values) == 0 {
		testResult.Reason = fmt.Errorf("no metric values found for %s - credentials may not be working", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

// ValidateCredentialProvider verifies the expected credential provider was used
func ValidateCredentialProvider(expectedProvider string, expectedAccessKeyID string) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("ValidateCredentialProvider: %s", expectedProvider),
		Status: status.FAILED,
	}

	info, err := ParseAgentLogsForCredentialProvider(expectedProvider)
	if err != nil {
		testResult.Reason = fmt.Errorf("failed to parse agent logs: %w", err)
		return testResult
	}

	if info.ProviderName != expectedProvider {
		testResult.Reason = fmt.Errorf("expected provider %s but got %s", expectedProvider, info.ProviderName)
		return testResult
	}

	// Verify access key matches (first 4 characters)
	if len(expectedAccessKeyID) >= 4 && len(info.AccessKeyID) >= 4 {
		if expectedAccessKeyID != info.AccessKeyID {
			testResult.Reason = fmt.Errorf("access key mismatch: credentials did not match expected: expected: %s, actual: %s", expectedAccessKeyID, info.AccessKeyID)
			return testResult
		}
	} else {
		testResult.Reason = fmt.Errorf("insufficient access key length for comparison: expected %d, got %d", len(expectedAccessKeyID), len(info.AccessKeyID))
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

type ExpectedResults struct {
	Namespace              string
	MetricName             string
	CredentialProviderName string
	AccessKeyID            string
}

func ValidateCredentialTest(testName string, expected ExpectedResults, metadata *environment.MetaData) status.TestGroupResult {
	return status.TestGroupResult{
		Name: testName,
		TestResults: []status.TestResult{
			// Validate metric delivery (proves credentials worked)
			ValidateMetric(testName, expected.Namespace, expected.MetricName, metadata),
			// Validate credential provider (proves correct credential source was used)
			ValidateCredentialProvider(expected.CredentialProviderName, expected.AccessKeyID),
		},
	}
}

// ValidateIMDSv2Used checks if IMDSv2 was used for credential retrieval
func ValidateIMDSv2Used() (bool, error) {
	file, err := os.Open(common.AgentLogFile)
	if err != nil {
		return false, fmt.Errorf("failed to open agent log: %w", err)
	}
	defer file.Close()

	// Look for IMDSv2 token request patterns
	imdsv2Pattern := regexp.MustCompile(`IMDSv2|X-aws-ec2-metadata-token`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if imdsv2Pattern.MatchString(line) {
			return true, nil
		}
	}

	if err = scanner.Err(); err != nil {
		return false, fmt.Errorf("error reading agent log: %w", err)
	}

	return false, nil
}

// ValidateSTSEndpoint verifies which STS endpoint was used
func ValidateSTSEndpoint() (string, error) {
	file, err := os.Open(common.AgentLogFile)
	if err != nil {
		return "", fmt.Errorf("failed to open agent log: %w", err)
	}
	defer file.Close()

	// Pattern for STS endpoint usage
	stsPattern := regexp.MustCompile(`sts\.([a-z0-9-]+)\.amazonaws\.com`)

	var lastEndpoint string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := stsPattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			lastEndpoint = matches[1]
		}
	}

	if err = scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading agent log: %w", err)
	}

	if lastEndpoint == "" {
		return "", fmt.Errorf("no STS endpoint found in logs")
	}

	return lastEndpoint, nil
}
