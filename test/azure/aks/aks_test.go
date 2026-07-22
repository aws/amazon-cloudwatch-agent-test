// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package aks validates the agent on a real AKS cluster running default:otel: a load-generator Job
// pushes OTLP to the DaemonSet agent via hostNetwork, and this test validates metrics/logs/traces
// reach CloudWatch via the AKS projected-token → AWS STS web-identity federation chain.
package aks

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	otlpLogGroup  = "/aws/cwagent/otlp"
	spansLogGroup = "aws/spans"
	serviceName   = "aks-otlp-test-service"
	// The load generator runs for 3 minutes; allow extra ingestion time.
	validationWindow = 10 * time.Minute
)

var env *environment.MetaData

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	if env.InstanceId == "" {
		fmt.Fprintln(os.Stderr, "instanceId flag is required (AKS cluster name) to scope telemetry")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestAKS(t *testing.T) {
	t.Run("Metrics", func(t *testing.T) {
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"AKSDefaultOtel", env.Region, []string{"aks_otlp_counter"},
			map[string]string{
				"@resource.host.id":        env.InstanceId,
				"@resource.cloud.provider": "azure",
			},
		)
		for _, r := range group.TestResults {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s: %v", r.Name, r.Reason)
		}
	})

	t.Run("Logs", func(t *testing.T) {
		r := validateLogs()
		require.Equal(t, status.SUCCESSFUL, r.Status, "log validation failed: %v", r.Reason)
	})

	t.Run("Traces", func(t *testing.T) {
		r := validateTraces()
		require.Equal(t, status.SUCCESSFUL, r.Status, "trace validation failed: %v", r.Reason)
	})
}

func validateLogs() status.TestResult {
	testResult := status.TestResult{Name: "AKS_Logs", Status: status.FAILED}

	// The log group is shared with concurrent jobs (e.g. the AzureVM test), so only
	// streams named for this cluster or service count, and a stream must be non-empty:
	// AssertPerLog alone passes vacuously on a stream with no events in the window.
	marker := fmt.Sprintf("aks_otlp_log_%s", env.InstanceId)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		streams := awsservice.GetLogStreams(otlpLogGroup)
		testResult.Reason = fmt.Errorf("attempt %d: no log streams matching %q or %q in %s", attempt, env.InstanceId, serviceName, otlpLogGroup)
		since := time.Now().Add(-validationWindow)
		until := time.Now()
		for _, stream := range streams {
			name := aws.ToString(stream.LogStreamName)
			if !strings.Contains(name, env.InstanceId) && !strings.Contains(name, serviceName) {
				continue
			}
			log.Printf("[AKS_Logs] attempt %d: checking %s/%s", attempt, otlpLogGroup, name)
			err := awsservice.ValidateLogs(
				otlpLogGroup, name, &since, &until,
				awsservice.AssertLogsNotEmpty(),
				awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
			)
			if err == nil {
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = err
		}
		if attempt < maxRetries {
			log.Printf("[AKS_Logs] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// validateTraces queries aws/spans (Transaction Search) for spans with our cluster's service name.
func validateTraces() status.TestResult {
	testResult := status.TestResult{Name: "AKS_Traces", Status: status.FAILED}

	query := fmt.Sprintf(
		`fields traceId | filter @message like "%s" and @message like "%s" | dedup traceId | limit 5`,
		serviceName, env.InstanceId,
	)
	log.Printf("[AKS_Traces] querying %s for spans from service=%s instance=%s", spansLogGroup, serviceName, env.InstanceId)

	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		rows, err := awsservice.GetLogQueryResults(spansLogGroup, since.Unix(), time.Now().Unix(), query)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: %s query failed: %w", attempt, spansLogGroup, err)
		} else {
			found := 0
			for _, row := range rows {
				for _, field := range row {
					if aws.ToString(field.Field) == "traceId" && aws.ToString(field.Value) != "" {
						found++
					}
				}
			}
			if found > 0 {
				log.Printf("[AKS_Traces] attempt %d: found %d traces in %s", attempt, found, spansLogGroup)
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: 0 traces found in %s for service=%s", attempt, spansLogGroup, serviceName)
		}
		if attempt < maxRetries {
			log.Printf("[AKS_Traces] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// filterLogLines returns lines containing any of the given substrings (case-insensitive).
func filterLogLines(text string, substrs ...string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		for _, s := range substrs {
			if strings.Contains(lower, strings.ToLower(s)) {
				result = append(result, line)
				break
			}
		}
	}
	if len(result) > 50 {
		result = result[len(result)-50:]
	}
	return result
}
