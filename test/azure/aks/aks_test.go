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
	serviceName   = "aks-otlp-test-service"
	// The load generator runs for 3 minutes; allow extra ingestion time.
	validationWindow = 10 * time.Minute
)

var env *environment.MetaData

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	if env.AKSClusterName == "" {
		fmt.Fprintln(os.Stderr, "aksClusterName flag is required to scope telemetry to this cluster")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestAKS(t *testing.T) {
	t.Run("Metrics", func(t *testing.T) {
		// host.id is overwritten by resourcedetection with the node's VMSS instance ID,
		// so scope by the cluster-name resource attribute the load generator sends.
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"AKSDefaultOtel", env.Region, []string{"aks_otlp_counter"},
			map[string]string{
				"@resource.k8s.cluster.name": env.AKSClusterName,
				"@resource.cloud.provider":   "azure",
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

	// The agent's k8s logs routing derives the destination from the k8s.cluster.name and
	// k8s.namespace.name resource attributes the load generator sends, so it is unique to
	// this cluster. The stream is {k8s.namespace.name}/{service.namespace}/{service.name},
	// where the agent's identity transform fills service.namespace from k8s.namespace.name.
	// AssertLogsNotEmpty guards against a vacuous pass on an empty window.
	logGroup := fmt.Sprintf("/aws/cwagent/%s/otlp", env.AKSClusterName)
	logStream := fmt.Sprintf("amazon-cloudwatch/amazon-cloudwatch/%s", serviceName)
	marker := fmt.Sprintf("aks_otlp_log_%s", env.AKSClusterName)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		until := time.Now()
		log.Printf("[AKS_Logs] attempt %d: checking %s/%s", attempt, logGroup, logStream)
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
		serviceName, env.AKSClusterName,
	)
	log.Printf("[AKS_Traces] querying %s for spans from service=%s instance=%s", spansLogGroup, serviceName, env.AKSClusterName)

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
