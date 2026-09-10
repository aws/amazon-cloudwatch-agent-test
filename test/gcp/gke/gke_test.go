// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package gke validates the agent on a real GKE cluster running default:otel: a load-generator Job
// pushes OTLP to the DaemonSet agent via hostNetwork, and this test validates metrics/logs/traces
// reach CloudWatch via the GKE projected-token → AWS STS web-identity federation chain.
package gke

import (
	"flag"
	"fmt"
	"log"
	"os"
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
	spansLogGroup = "aws/spans"
	serviceName   = "gke-otlp-test-service"
	// The load generator runs for 3 minutes; allow extra ingestion time.
	validationWindow = 10 * time.Minute
)

var env *environment.MetaData

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	if env.GKEClusterName == "" {
		fmt.Fprintln(os.Stderr, "gkeClusterName flag is required to scope telemetry to this cluster")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestGKE(t *testing.T) {
	t.Run("Metrics", func(t *testing.T) {
		// test_id is a datapoint attribute, the one surface no resource processor rewrites, so it
		// isolates this run. cloud.platform=gcp_kubernetes_engine comes only from the gcp detector:
		// proves detection ran.
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"GKEDefaultOtel", env.Region, []string{"gke_otlp_counter"},
			map[string]string{
				"test_id":                  env.GKEClusterName,
				"@resource.cloud.platform": "gcp_kubernetes_engine",
				"@resource.cloud.provider": "gcp",
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
	testResult := status.TestResult{Name: "GKE_Logs", Status: status.FAILED}

	// The agent's k8s logs routing derives the destination from the k8s.cluster.name and
	// k8s.namespace.name resource attributes the load generator sends, so it is unique to
	// this cluster. The stream is {k8s.namespace.name}/{service.namespace}/{service.name},
	// where the agent's identity transform fills service.namespace from k8s.namespace.name.
	// AssertLogsNotEmpty guards against a vacuous pass on an empty window.
	logGroup := fmt.Sprintf("/aws/cwagent/%s/otlp", env.GKEClusterName)
	// Clean up only on success: the group name carries this run's cluster so the whole group is
	// disposable, but on failure it is left in place as evidence for whoever debugs the run.
	defer func() {
		if testResult.Status == status.SUCCESSFUL {
			awsservice.DeleteLogGroup(logGroup)
		}
	}()
	logStream := fmt.Sprintf("amazon-cloudwatch/amazon-cloudwatch/%s", serviceName)
	marker := fmt.Sprintf("gke_otlp_log_%s", env.GKEClusterName)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		until := time.Now()
		log.Printf("[GKE_Logs] attempt %d: checking %s/%s", attempt, logGroup, logStream)
		err := awsservice.ValidateLogs(
			logGroup, logStream, &since, &until,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
		)
		if err == nil {
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		testResult.Reason = err
		if attempt < maxRetries {
			log.Printf("[GKE_Logs] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// validateTraces queries aws/spans (Transaction Search) for spans with our cluster's service name.
func validateTraces() status.TestResult {
	testResult := status.TestResult{Name: "GKE_Traces", Status: status.FAILED}

	query := fmt.Sprintf(
		`fields traceId | filter @message like "%s" and @message like "%s" | dedup traceId | limit 5`,
		serviceName, env.GKEClusterName,
	)
	log.Printf("[GKE_Traces] querying %s for spans from service=%s instance=%s", spansLogGroup, serviceName, env.GKEClusterName)

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
				log.Printf("[GKE_Traces] attempt %d: found %d traces in %s", attempt, found, spansLogGroup)
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: 0 traces found in %s for service=%s", attempt, spansLogGroup, serviceName)
		}
		if attempt < maxRetries {
			log.Printf("[GKE_Traces] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}
