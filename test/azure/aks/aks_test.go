// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package aks validates the agent on a real AKS cluster running default:otel: a load-generator Job
// pushes OTLP to the DaemonSet agent via hostNetwork, and this test validates metrics/logs/traces
// reach CloudWatch via the AKS projected-token → AWS STS web-identity federation chain.
package aks

import (
	"encoding/json"
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
	// agentNamespace is the k8s namespace the agent runs in. The resource k8s.namespace.name and the
	// derived service.namespace both carry it.
	agentNamespace = "amazon-cloudwatch"
	// The load generator runs for 3 minutes; allow extra ingestion time.
	validationWindow = 10 * time.Minute
)

var env *environment.MetaData

// aksResourceExpectations is the shared source of truth for the azure.aks resourcedetection attributes
// asserted on every signal: an exact value, or PresenceOnly for present-and-non-empty. Node-level and
// subscription-bearing attributes stay presence-only (their values are per-node/dynamic, or must be kept
// out of the CI logs).
func aksResourceExpectations() map[string]string {
	return map[string]string{
		"cloud.provider":              "azure",
		"cloud.platform":              "azure.aks",
		"k8s.cluster.name":            env.AKSClusterName,
		"k8s.namespace.name":          agentNamespace,
		"service.namespace":           agentNamespace,
		"deployment.environment.name": "azure.aks:" + env.AKSClusterName + "/" + agentNamespace,
		"cloud.region":                env.AzureLocation,
		"azure.vm.size":               env.AzureVMSize,        // AKS node pool VM size
		"azure.resourcegroup.name":    env.AzureResourceGroup, // AKS node resource group (MC_...)
		"cloud.account.id":            otlpvalidation.PresenceOnly,
		"cloud.resource_id":           otlpvalidation.PresenceOnly,
		// Per-node VMSS-generated names / machine id: not knowable at plan time.
		"host.id":                otlpvalidation.PresenceOnly,
		"host.name":              otlpvalidation.PresenceOnly,
		"azure.vm.name":          otlpvalidation.PresenceOnly,
		"azure.vm.scaleset.name": otlpvalidation.PresenceOnly,
		"service.name":           otlpvalidation.PresenceOnly,
	}
}

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
		// test_id (a datapoint attribute no resource processor rewrites) isolates this run. The @resource.*
		// labels prove the azure.aks resourcedetection enrichment.
		labels := map[string]string{"test_id": env.AKSClusterName}
		for attr, want := range aksResourceExpectations() {
			labels["@resource."+attr] = otlpvalidation.ExpectedValue(want)
		}
		for attr, want := range otlpvalidation.ScopeExpectations() {
			labels["@instrumentation."+attr] = otlpvalidation.ExpectedValue(want)
		}
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"AKSDefaultOtel", env.Region, []string{"aks_otlp_counter"}, labels)
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
	// Clean up only on success: the group name carries this run's cluster so the whole group is
	// disposable, but on failure it is left in place as evidence for whoever debugs the run.
	defer func() {
		if testResult.Status == status.SUCCESSFUL {
			awsservice.DeleteLogGroup(logGroup)
		}
	}()
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
			awsservice.AssertPerLog(otlpvalidation.AssertLogRecord(func(rec otlpvalidation.LogRecord) error {
				return otlpvalidation.AssertLogContent(rec, marker, "INFO", aksResourceExpectations())
			})),
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

// validateTraces queries aws/spans (Transaction Search) for this run's spans. The load generator is an
// external k8s Job, so we cannot enumerate trace IDs. Match spans by service name + cluster instead, then
// assert the azure.aks resource enrichment on each matched span (not just that some arrived).
func validateTraces() status.TestResult {
	testResult := status.TestResult{Name: "AKS_Traces", Status: status.FAILED}

	query := fmt.Sprintf(
		`fields @message | filter @message like "%s" and @message like "%s" | limit 100`,
		serviceName, env.AKSClusterName,
	)
	log.Printf("[AKS_Traces] querying %s for spans from service=%s cluster=%s", spansLogGroup, serviceName, env.AKSClusterName)

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
					if aws.ToString(field.Field) != "@message" {
						continue
					}
					var s otlpvalidation.SpanRecord
					if json.Unmarshal([]byte(aws.ToString(field.Value)), &s) != nil {
						continue
					}
					// Content is stable, so a mismatch is final. Fail immediately.
					if cerr := otlpvalidation.AssertAttributes("span "+s.TraceID, aksResourceExpectations(), s.Resource.Attributes); cerr != nil {
						testResult.Reason = cerr
						return testResult
					}
					found++
				}
			}
			if found > 0 {
				log.Printf("[AKS_Traces] attempt %d: %d spans found with expected content in %s", attempt, found, spansLogGroup)
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: 0 spans found in %s for service=%s", attempt, spansLogGroup, serviceName)
		}
		if attempt < maxRetries {
			log.Printf("[AKS_Traces] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}
