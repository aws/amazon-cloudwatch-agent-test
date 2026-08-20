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
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// DynamoDB table identifiers and regression thresholds.
const (
	tableName            = "CWAPerformanceMetrics"
	useCase              = "otel-containerinsights"
	serviceName          = "AmazonCloudWatchAgent"
	maxRegressionPercent = 30.0
	// A drop larger than this fails too: a big decrease is either breakage/a
	// missing metric or a large optimization — both warrant a human look and a
	// deliberate re-baseline rather than silently lowering the bar.
	maxDropPercent = 50.0
)

// PerfResult stores the worst-case (max) performance values for a run.
type PerfResult struct {
	DaemonSetMemMaxMB float64 `dynamodbav:"daemonset_mem_max_mb"`
	DaemonSetCPUMax   float64 `dynamodbav:"daemonset_cpu_max"`
	ScraperMemMaxMB   float64 `dynamodbav:"scraper_mem_max_mb"`
	ScraperCPUMax     float64 `dynamodbav:"scraper_cpu_max"`
}

// TestRegressionCheck queries current agent resource usage, compares it against
// the previous run for the same commit and instance type, and stores the
// baseline only when the comparison passes.
func TestRegressionCheck(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	commitHash := env.CwaCommitSha
	// Require a real commit sha. Falling back to the cluster name broke the
	// comparison: fetchPreviousResult excludes rows whose hash matches the
	// current one, so a cluster-name hash excluded every prior row from the
	// same cluster and the test always logged "first run" and passed.
	require.NotEmpty(t, commitHash, "cwaCommitSha is required for the regression baseline; pass -cwaCommitSha so runs compare against the same commit")

	metrics := fetchSharedMetrics(t)
	instanceType := resolveInstanceType(t, metrics)
	t.Logf("Instance type: %s", instanceType)

	current := collectCurrentResults(t)
	previous, prevCommit, hasPrevious := fetchPreviousResult(t, commitHash, instanceType)
	if !hasPrevious {
		t.Logf("First run for instance type %s — nothing to compare against. Storing baseline.", instanceType)
		storeResult(t, commitHash, instanceType, current)
		return
	}
	t.Logf("Comparing current (commit: %s) to previous (commit: %s)", commitHash, prevCommit)
	passed := true
	passed = compareAndReport(t, "DaemonSet memory", previous.DaemonSetMemMaxMB, current.DaemonSetMemMaxMB, "MB") && passed
	passed = compareAndReport(t, "Cluster scraper memory", previous.ScraperMemMaxMB, current.ScraperMemMaxMB, "MB") && passed
	passed = compareAndReport(t, "DaemonSet CPU", previous.DaemonSetCPUMax, current.DaemonSetCPUMax, "cores") && passed
	passed = compareAndReport(t, "Cluster scraper CPU", previous.ScraperCPUMax, current.ScraperCPUMax, "cores") && passed

	// Only update the baseline on a passing run. Persisting a regressed result
	// would let the next commit compare against the inflated values and pass,
	// resetting the bar after a single red run.
	if !passed {
		t.Log("Regression detected — leaving the stored baseline unchanged.")
		return
	}
	storeResult(t, commitHash, instanceType, current)
}

// ANSI color codes for terminal output.
const (
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

// compareAndReport logs the comparison and reports whether it passed. A regression above the
// threshold fails the test; a zero baseline is treated as a corrupt/missing
// row and also fails, since agent usage is never legitimately zero.
func compareAndReport(t *testing.T, metricName string, previous, current float64, unit string) bool {
	t.Helper()
	if previous == 0 {
		t.Errorf("  %s: previous baseline is 0 — missing or corrupt baseline row, cannot compare.", metricName)
		t.Log("")
		return false
	}
	changePercent := ((current - previous) / previous) * 100
	if changePercent <= 0 {
		dropPercent := -changePercent
		if dropPercent > maxDropPercent {
			t.Logf("  Current %s usage is %.1f%% %sLESS%s than last known usage (%.4f %s -> %.4f %s)",
				metricName, dropPercent, colorRed, colorReset, previous, unit, current, unit)
			t.Errorf("    %.1f%% drop > %.0f%% drop threshold — investigate (breakage/missing metric) or re-baseline if intentional, Regression Test: %sFAIL%s",
				dropPercent, maxDropPercent, colorRed, colorReset)
			t.Log("")
			return false
		}
		t.Logf("  Current %s usage is %.1f%% %sLESS%s than last known usage (%.4f %s -> %.4f %s)",
			metricName, dropPercent, colorGreen, colorReset, previous, unit, current, unit)
		t.Logf("    Regression Test: %sPASS%s", colorGreen, colorReset)
		t.Log("")
		return true
	} else if changePercent > maxRegressionPercent {
		t.Logf("  Current %s usage is %.1f%% %sMORE%s than last known usage (%.4f %s -> %.4f %s)",
			metricName, changePercent, colorRed, colorReset, previous, unit, current, unit)
		t.Errorf("    %.1f%% growth > %.0f%% growth threshold, Regression Test: %sFAIL%s",
			changePercent, maxRegressionPercent, colorRed, colorReset)
		t.Log("")
		return false
	}
	t.Logf("  Current %s usage is %.1f%% %sMORE%s than last known usage (%.4f %s -> %.4f %s)",
		metricName, changePercent, colorRed, colorReset, previous, unit, current, unit)
	t.Logf("    %.1f%% growth <= %.0f%% growth threshold, Regression Test: %sPASS%s",
		changePercent, maxRegressionPercent, colorGreen, colorReset)
	t.Log("")
	return true
}

// resolveInstanceType resolves the node instance type for this run from the pod
// series' host.type resource label. The %-of-node thresholds are calibrated for
// one instance type, so a missing label or a mix of types is a hard failure.
func resolveInstanceType(t *testing.T, metrics *podMetricData) string {
	t.Helper()
	seen := make(map[string]struct{})
	for _, series := range metrics.CPUResults {
		hostType := series.Labels.Resource["host.type"]
		require.NotEmpty(t, hostType, "CPU series is missing the host.type resource label — cannot determine instance type")
		seen[hostType] = struct{}{}
	}
	for _, series := range metrics.MemResults {
		hostType := series.Labels.Resource["host.type"]
		require.NotEmpty(t, hostType, "memory series is missing the host.type resource label — cannot determine instance type")
		seen[hostType] = struct{}{}
	}
	require.Len(t, seen, 1, "expected a single node instance type across agent pods, found %d: %v — thresholds are calibrated for one type", len(seen), seen)
	for hostType := range seen {
		return hostType
	}
	return ""
}

// collectCurrentResults finds the max CPU/mem across all pods, returning the
// worst-case values for each category.
func collectCurrentResults(t *testing.T) PerfResult {
	t.Helper()
	metrics := fetchSharedMetrics(t)
	require.NotEmpty(t, metrics.CPUResults, "no CPU data")
	require.NotEmpty(t, metrics.MemResults, "no memory data")
	var result PerfResult
	var sawDaemonSet, sawScraper bool
	for _, series := range metrics.CPUResults {
		podName := series.Labels.Resource["k8s.pod.name"]
		require.NotEmpty(t, podName, "series is missing the k8s.pod.name resource label")
		_, max := calcStats(series.Values)
		if isDaemonSetPod(podName) {
			sawDaemonSet = true
			if max > result.DaemonSetCPUMax {
				result.DaemonSetCPUMax = max
			}
		} else {
			sawScraper = true
			if max > result.ScraperCPUMax {
				result.ScraperCPUMax = max
			}
		}
	}
	for _, series := range metrics.MemResults {
		podName := series.Labels.Resource["k8s.pod.name"]
		require.NotEmpty(t, podName, "series is missing the k8s.pod.name resource label")
		_, max := calcStats(series.Values)
		maxMB := max / (1024 * 1024)
		if isDaemonSetPod(podName) {
			sawDaemonSet = true
			if maxMB > result.DaemonSetMemMaxMB {
				result.DaemonSetMemMaxMB = maxMB
			}
		} else {
			sawScraper = true
			if maxMB > result.ScraperMemMaxMB {
				result.ScraperMemMaxMB = maxMB
			}
		}
	}

	// A missing class would otherwise stay at zero and read as "reduced usage"
	// against a positive baseline — require both classes to be present.
	require.True(t, sawDaemonSet, "no DaemonSet pod series observed — expected both pod classes to be present")
	require.True(t, sawScraper, "no cluster-scraper pod series observed — expected both pod classes to be present")
	return result
}

// fetchPreviousResult returns the latest previous result for a different commit
// on the same instance type, its commit hash, and whether one was found.
func fetchPreviousResult(t *testing.T, currentCommitHash, instanceType string) (PerfResult, string, bool) {
	t.Helper()
	data, err := awsservice.DynamodbClient.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("UseCaseDate"),
		KeyConditionExpression: aws.String("#uc = :uc"),
		FilterExpression:       aws.String("#ch <> :ch AND #it = :it"),
		ExpressionAttributeNames: map[string]string{
			"#uc": "UseCase",
			"#ch": "CommitHash",
			"#it": "InstanceType",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":uc": &types.AttributeValueMemberS{Value: useCase},
			":ch": &types.AttributeValueMemberS{Value: currentCommitHash},
			":it": &types.AttributeValueMemberS{Value: instanceType},
		},
		ScanIndexForward: aws.Bool(false),
	})
	// A query error (missing table, permissions, throttling) is an
	// infrastructure failure, not a "first run" — fail loudly rather than
	// silently passing the regression check.
	require.NoError(t, err, "querying previous results from DynamoDB table %s", tableName)

	if len(data.Items) == 0 {
		return PerfResult{}, "", false
	}
	var record map[string]interface{}
	err = attributevalue.UnmarshalMap(data.Items[0], &record)
	require.NoError(t, err, "unmarshalling previous result row")

	results, ok := record["Results"].(map[string]interface{})
	require.True(t, ok, "previous result row is missing the Results field — corrupt row?")
	prevCommit, _ := record["CommitHash"].(string)
	return PerfResult{
		DaemonSetMemMaxMB: getFloat(results, "daemonset_mem_max_mb"),
		DaemonSetCPUMax:   getFloat(results, "daemonset_cpu_max"),
		ScraperMemMaxMB:   getFloat(results, "scraper_mem_max_mb"),
		ScraperCPUMax:     getFloat(results, "scraper_cpu_max"),
	}, prevCommit, true
}

// storeResult writes the current run's results to DynamoDB.
func storeResult(t *testing.T, commitHash, instanceType string, result PerfResult) {
	t.Helper()

	resultsMap, err := attributevalue.MarshalMap(result)
	require.NoError(t, err, "%sCOULD NOT SAVE VALUES%s: failed to marshal results", colorRed, colorReset)
	item := map[string]types.AttributeValue{
		"Service": &types.AttributeValueMemberS{Value: serviceName},
		// Deterministic key (no timestamp) so a re-run of the same commit and
		// instance type overwrites its row instead of appending, keeping one
		// baseline row per commit+instance. instanceType is included so the same
		// commit measured on different node types doesn't clobber itself.
		"UniqueID":     &types.AttributeValueMemberS{Value: fmt.Sprintf("%s-%s-%s", useCase, commitHash, instanceType)},
		"UseCase":      &types.AttributeValueMemberS{Value: useCase},
		"CommitHash":   &types.AttributeValueMemberS{Value: commitHash},
		"CommitDate":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		"Results":      &types.AttributeValueMemberM{Value: resultsMap},
		"ClusterName":  &types.AttributeValueMemberS{Value: cfg.ClusterName},
		"InstanceType": &types.AttributeValueMemberS{Value: instanceType},
	}
	// Retry transient DynamoDB errors (throttling, blips) rather than failing an
	// otherwise-passing run, matching the performance validator's write pattern.
	err = backoff.Retry(func() error {
		_, putErr := awsservice.DynamodbClient.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      item,
		})
		return putErr
	}, awsservice.StandardExponentialBackoff)
	require.NoError(t, err, "%sCOULD NOT SAVE VALUES%s", colorRed, colorReset)
}

// isDaemonSetPod reports whether the pod name belongs to the DaemonSet (not the cluster scraper).
func isDaemonSetPod(podName string) bool {
	return len(podName) > 0 && !strings.Contains(podName, "cluster-scraper")
}

// getFloat safely extracts a float64 from a map[string]interface{}.
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
