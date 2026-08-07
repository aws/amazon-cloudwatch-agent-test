//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)


// define useful constants 
const (
	tableName = "CWAPerformanceMetrics"
	useCase = "otel-containerinsights"
	serviceName = "AmazonCloudWatchAgent"
	maxRegressionPercent = 30.0
)


// struct to store max performance values
type PerfResult struct {
	DaemonSetMemMaxMB float64 `dynamodbav:"daemonset_mem_max_mb"`
	DaemonSetCPUMax   float64 `dynamodbav:"daemonset_cpu_max"`
	ScraperMemMaxMB   float64 `dynamodbav:"scraper_mem_max_mb"`
	ScraperCPUMax     float64 `dynamodbav:"scraper_cpu_max"`
}


// Query current agent resource usage, store the results, compare against prev runs. 
func TestRegressionCheck(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	commitHash := env.CwaCommitSha
	if commitHash == "" {
		commitHash = cfg.ClusterName 
	}
	current := collectCurrentResults(t)
	storeResult(t, commitHash, current)
	previous, prevCommit, hasPrevious := fetchPreviousResult(t, commitHash)
	if !hasPrevious {
		t.Log("First run — nothing to compare against. Skipping regression comparison.")
		return
	}
	t.Logf("Comparing current (commit: %s) to previous (commit: %s)", commitHash, prevCommit)
	compareAndReport(t, "DaemonSet memory", previous.DaemonSetMemMaxMB, current.DaemonSetMemMaxMB, "MB")
	compareAndReport(t, "Cluster scraper memory", previous.ScraperMemMaxMB, current.ScraperMemMaxMB, "MB")
	compareAndReport(t, "DaemonSet CPU", previous.DaemonSetCPUMax, current.DaemonSetCPUMax, "cores")
	compareAndReport(t, "Cluster scraper CPU", previous.ScraperCPUMax, current.ScraperCPUMax, "cores")
}


// ANSI color codes for terminal output.
const (
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)


// Log the comparison and fail if regression exceeds threshold.
func compareAndReport(t *testing.T, metricName string, previous, current float64, unit string) {
	t.Helper()
	if previous == 0 {
		t.Logf("  %s: previous value is 0, skipping regression comparison.", metricName)
		return
	}
	changePercent := ((current - previous) / previous) * 100
	if changePercent <= 0 {
		t.Logf("  Current %s usage is %.1f%% %sLESS%s than last known usage (%.4f %s -> %.4f %s)",
			metricName, -changePercent, colorGreen, colorReset, previous, unit, current, unit)
		t.Logf("    Regression Test: %sPASS%s", colorGreen, colorReset)
	} else if changePercent > maxRegressionPercent {
		t.Logf("  Current %s usage is %.1f%% %sMORE%s than last known usage (%.4f %s -> %.4f %s)",
			metricName, changePercent, colorRed, colorReset, previous, unit, current, unit)
		t.Errorf("    %.1f%% growth > %.0f%% growth threshold, Regression Test: %sFAIL%s",
			changePercent, maxRegressionPercent, colorRed, colorReset)
	} else {
		t.Logf("  Current %s usage is %.1f%% %sMORE%s than last known usage (%.4f %s -> %.4f %s)",
			metricName, changePercent, colorRed, colorReset, previous, unit, current, unit)
		t.Logf("    %.1f%% growth <= %.0f%% growth threshold, Regression Test: %sPASS%s",
			changePercent, maxRegressionPercent, colorGreen, colorReset)
	}
	t.Log("")
}


//Find the max CPU/ mem accross all pods. Return worst case values for each category. 
func collectCurrentResults(t *testing.T) PerfResult {
	t.Helper()
	metrics := fetchSharedMetrics(t)
	require.NotEmpty(t, metrics.CPUResults, "no CPU data")
	require.NotEmpty(t, metrics.MemResults, "no memory data")
	var result PerfResult
	for _, series := range metrics.CPUResults {
		podName := series.Labels.Resource["k8s.pod.name"]
		_, max := calcStats(series.Values)
		if isDaemonSetPod(podName) {
			if max > result.DaemonSetCPUMax {
				result.DaemonSetCPUMax = max
			}
		} else {
			result.ScraperCPUMax = max
		}
	}
	for _, series := range metrics.MemResults {
		podName := series.Labels.Resource["k8s.pod.name"]
		_, max := calcStats(series.Values)
		maxMB := max / (1024 * 1024)
		if isDaemonSetPod(podName) {
			if maxMB > result.DaemonSetMemMaxMB {
				result.DaemonSetMemMaxMB = maxMB
			}
		} else {
			result.ScraperMemMaxMB = maxMB
		}
	}
	return result
}


// Returns latest previous result, the commit hash of that result, if one was found.
func fetchPreviousResult(t *testing.T, currentCommitHash string) (PerfResult, string, bool) {
	t.Helper()
	data, err := awsservice.DynamodbClient.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("UseCaseDate"),
		KeyConditionExpression: aws.String("#uc = :uc"),
		FilterExpression:       aws.String("#ch <> :ch"),
		ExpressionAttributeNames: map[string]string{
			"#uc": "UseCase",
			"#ch": "CommitHash",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":uc": &types.AttributeValueMemberS{Value: useCase},
			":ch": &types.AttributeValueMemberS{Value: currentCommitHash},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		t.Logf("DynamoDB query error (non-fatal): %v", err)
		return PerfResult{}, "", false
	}
	if len(data.Items) == 0 {
		return PerfResult{}, "", false
	}
	var record map[string]interface{}
	if err := attributevalue.UnmarshalMap(data.Items[0], &record); err != nil {
		t.Logf("Failed to unmarshal previous result: %v", err)
		return PerfResult{}, "", false
	}
	results, ok := record["Results"].(map[string]interface{})
	if !ok {
		return PerfResult{}, "", false
	}
	prevCommit, _ := record["CommitHash"].(string)
	return PerfResult{
		DaemonSetMemMaxMB: getFloat(results, "daemonset_mem_max_mb"),
		DaemonSetCPUMax:   getFloat(results, "daemonset_cpu_max"),
		ScraperMemMaxMB:   getFloat(results, "scraper_mem_max_mb"),
		ScraperCPUMax:     getFloat(results, "scraper_cpu_max"),
	}, prevCommit, true
}

// Write the current run's results to DynamoDB.
func storeResult(t *testing.T, commitHash string, result PerfResult) {
	t.Helper()

	resultsMap, err := attributevalue.MarshalMap(result)
	if err != nil {
		t.Errorf("%sCOULD NOT SAVE VALUES%s: failed to marshal results: %v", colorRed, colorReset, err)
		return
	}
	item := map[string]types.AttributeValue{
		"Service":     &types.AttributeValueMemberS{Value: serviceName},
		"UniqueID":    &types.AttributeValueMemberS{Value: fmt.Sprintf("%s-%s-%d", useCase, commitHash, time.Now().Unix())},
		"UseCase":     &types.AttributeValueMemberS{Value: useCase},
		"CommitHash":  &types.AttributeValueMemberS{Value: commitHash},
		"CommitDate":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		"Results":     &types.AttributeValueMemberM{Value: resultsMap},
		"ClusterName": &types.AttributeValueMemberS{Value: cfg.ClusterName},
	}
	_, err = awsservice.DynamodbClient.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	if err != nil {
		t.Errorf("%sCOULD NOT SAVE VALUES%s: %v", colorRed, colorReset, err)
		return
	}
}


// Return true if the pod name belongs to the DaemonSet (not the cluster scraper).
func isDaemonSetPod(podName string) bool {
	return len(podName) > 0 && !strings.Contains(podName, "cluster-scraper")
}

// Safely extract a float64 from a map[string]interface{}.
func getFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
