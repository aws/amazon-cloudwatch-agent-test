// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

package containerinsights

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// The OTLP sender in resources/otlp_sender.yaml: an ordinary pod that sends through the
// cloudwatch-agent Service to the chart's OTLP endpoint, with the app.kubernetes.io/name and version
// labels and no service.name. The chart's otelServiceIdentity.useAppLabels is false by default, so
// those labels are not a source of service.name or service.version: service.name is the Deployment name.
const (
	otlpNamespace = "otlp-e2e"
	otlpService   = "otlp-e2e-app"
	otlpNameLabel = "otlp-e2e-app-name"
	otlpCase      = "app"
)

// otlpAbsentKeys are resource attributes the sender's logs and spans must not have: its
// app.kubernetes.io/version label is not used for service.version by default.
var otlpAbsentKeys = []string{"service.version"}

// assertOTLPAbsent checks that attrs has none of otlpAbsentKeys.
func assertOTLPAbsent(scope string, attrs map[string]any) error {
	for _, key := range otlpAbsentKeys {
		if _, ok := attrs[key]; ok {
			return fmt.Errorf("%s: attribute %q must not be set", scope, key)
		}
	}
	return nil
}

// otlpMetricNames is every metric the chart's OTLP pipelines export for this cluster: the sender's
// counter and the span metrics computed from its spans.
var otlpMetricNames = []string{otlpendpoint.MetricName, "traces.span.metrics.calls", "traces.span.metrics.duration"}

// otlpResourceExpectations are the resource attributes of the sender's telemetry: the AKS resource
// detection of the node, the Container Insights enrichment of the pod, its labels and the service
// identity. Node-level and subscription-bearing values are presence-only, as in the default:otel
// suite. cloud.resource_id carries the subscription and is checked separately, without logging it.
func otlpResourceExpectations() map[string]string {
	return map[string]string{
		"cloud.provider":                       "azure",
		"cloud.platform":                       "azure.aks",
		"cloud.region":                         env.AzureLocation,
		"azure.vm.size":                        env.AzureVMSize,
		"azure.resourcegroup.name":             env.AzureResourceGroup,
		"k8s.cluster.name":                     env.AKSClusterName,
		"k8s.namespace.name":                   otlpNamespace,
		"k8s.deployment.name":                  otlpService,
		"k8s.workload.name":                    otlpService,
		"k8s.workload.type":                    "Deployment",
		"k8s.container.name":                   "sender",
		"k8s.pod.label.app":                    otlpService,
		"k8s.pod.label.app.kubernetes.io/name": otlpNameLabel,
		"k8s.node.label.kubernetes.io/os":      "linux",
		"service.name":                         otlpService,
		"service.namespace":                    otlpNamespace,
		"deployment.environment.name":          "azure.aks:" + env.AKSClusterName + "/" + otlpNamespace,
		"cloud.account.id":                     otlpvalidation.PresenceOnly,
		"cloud.resource_id":                    otlpvalidation.PresenceOnly,
		"k8s.pod.name":                         otlpvalidation.PresenceOnly,
		"k8s.pod.uid":                          otlpvalidation.PresenceOnly,
		"k8s.node.name":                        otlpvalidation.PresenceOnly,
		"service.instance.id":                  otlpvalidation.PresenceOnly,
		"host.id":                              otlpvalidation.PresenceOnly,
		"host.name":                            otlpvalidation.PresenceOnly,
		"azure.vm.name":                        otlpvalidation.PresenceOnly,
		"azure.vm.scaleset.name":               otlpvalidation.PresenceOnly,
	}
}

// TestAKSOTLP validates the chart's OTLP endpoint, which the chart serves by default, with the install
// values the AKS setup script uses (the chart ignores agents[].config default:otel and builds the
// default config). The sender's metrics, logs and traces must carry the Container Insights enrichment
// of its pod. Every check polls until the shared deadline, as validateMetrics does.
func TestAKSOTLP(t *testing.T) {
	deadline := time.Now().Add(validationWindow)
	const pollInterval = 15 * time.Second
	resourceLabels := map[string]string{}
	for attr, want := range otlpResourceExpectations() {
		resourceLabels["@resource."+attr] = otlpvalidation.ExpectedValue(want)
	}
	scopeLabels := map[string]string{
		"@instrumentation.cloudwatch.source":   otlpvalidation.ExactMatch(otlpvalidation.CloudWatchSource),
		"@instrumentation.cloudwatch.solution": otlpvalidation.ExactMatch(otlpendpoint.Solution),
	}
	withScope := func(extra map[string]string) map[string]string {
		labels := map[string]string{}
		for _, m := range []map[string]string{resourceLabels, scopeLabels, extra} {
			for k, v := range m {
				labels[k] = v
			}
		}
		return labels
	}
	// assertFound polls validate until every metric is found or the deadline passes.
	assertFound := func(t *testing.T, validate func() status.TestGroupResult) {
		var group status.TestGroupResult
		for {
			group = validate()
			ok := len(group.TestResults) > 0
			for _, r := range group.TestResults {
				ok = ok && r.Status == status.SUCCESSFUL
			}
			if ok || !time.Now().Before(deadline) {
				break
			}
			time.Sleep(pollInterval)
		}
		require.NotEmpty(t, group.TestResults)
		for _, r := range group.TestResults {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s: %v", r.Name, r.Reason)
		}
	}

	t.Run("Metrics", func(t *testing.T) {
		assertFound(t, func() status.TestGroupResult {
			return otlpvalidation.ValidateOtlpMetricsWithLabels("AKSOTLPMetric", env.Region,
				[]string{otlpendpoint.MetricName},
				withScope(map[string]string{otlpendpoint.CaseAttribute: otlpvalidation.ExactMatch(otlpCase)}))
		})
	})
	t.Run("SpanMetrics", func(t *testing.T) {
		assertFound(t, func() status.TestGroupResult {
			return otlpvalidation.ValidateOtlpMetricsWithLabels("AKSOTLPSpanMetrics", env.Region,
				otlpMetricNames[1:],
				withScope(map[string]string{"http.request.method": otlpvalidation.ExactMatch("GET")}))
		})
	})
	t.Run("Names", func(t *testing.T) {
		matcher := fmt.Sprintf(`{__name__=~".+","@resource.k8s.cluster.name"=%q,"@instrumentation.cloudwatch.solution"=%q}`,
			env.AKSClusterName, otlpendpoint.Solution)
		var got []string
		for {
			names, err := otlpvalidation.MetricNamesForMatcher(env.Region, matcher, time.Now().Add(-time.Hour), time.Now())
			require.NoError(t, err)
			got = names
			if len(got) >= len(otlpMetricNames) || !time.Now().Before(deadline) {
				break
			}
			time.Sleep(pollInterval)
		}
		require.ElementsMatch(t, otlpMetricNames, got)
	})
	t.Run("Logs", func(t *testing.T) {
		r := validateOTLPLogs()
		require.Equal(t, status.SUCCESSFUL, r.Status, "OTLP log validation failed: %v", r.Reason)
	})
	t.Run("Traces", func(t *testing.T) {
		r := validateOTLPTraces()
		require.Equal(t, status.SUCCESSFUL, r.Status, "OTLP trace validation failed: %v", r.Reason)
	})
}

// assertResourceID checks cloud.resource_id without putting the subscription in the error.
func assertResourceID(scope string, attrs map[string]any) error {
	want := "/providers/Microsoft.ContainerService/managedClusters/" + env.AKSClusterName
	got := fmt.Sprint(attrs["cloud.resource_id"])
	if !strings.HasPrefix(got, "/subscriptions/") || !strings.HasSuffix(got, want) {
		return fmt.Errorf("%s: cloud.resource_id is not the AKS cluster's resource ID (want /subscriptions/...%s)", scope, want)
	}
	return nil
}

func validateOTLPLogs() status.TestResult {
	testResult := status.TestResult{Name: "AKS_OTLP_Logs", Status: status.FAILED}
	logGroup := otlpendpoint.LogGroup(env.AKSClusterName)
	defer awsservice.DeleteLogGroup(logGroup)
	// {k8s.namespace.name}/{service.namespace}/{service.name}, with the identity the chart derives.
	logStream := strings.Join([]string{otlpNamespace, otlpNamespace, otlpService}, "/")
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		until := time.Now()
		err := awsservice.ValidateLogs(
			logGroup, logStream, &since, &until,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(otlpvalidation.AssertLogRecord(func(rec otlpvalidation.LogRecord) error {
				if err := otlpvalidation.AssertLogContent(rec, otlpendpoint.LogBody(otlpCase), "INFO", otlpResourceExpectations()); err != nil {
					return err
				}
				if err := assertOTLPAbsent("log", rec.Resource.Attributes); err != nil {
					return err
				}
				return assertResourceID("log", rec.Resource.Attributes)
			})),
		)
		if err == nil {
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		testResult.Reason = err
		if attempt < maxRetries {
			log.Printf("[AKS_OTLP_Logs] %v, retrying in %v", err, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

func validateOTLPTraces() status.TestResult {
	testResult := status.TestResult{Name: "AKS_OTLP_Traces", Status: status.FAILED}
	query := fmt.Sprintf(`fields @message | filter @message like %q and @message like %q | limit 100`,
		otlpService, env.AKSClusterName)
	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-validationWindow)
		rows, err := awsservice.GetLogQueryResults("aws/spans", since.Unix(), time.Now().Unix(), query)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: aws/spans query failed: %w", attempt, err)
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
					// Content is stable, so a mismatch is final.
					attrs := map[string]string{otlpendpoint.CaseAttribute: otlpCase, "http.request.method": "GET"}
					if cerr := otlpvalidation.AssertSpanContent(s, "GET /e2e", "SERVER", attrs, otlpResourceExpectations()); cerr != nil {
						testResult.Reason = cerr
						return testResult
					}
					if cerr := assertOTLPAbsent("span "+s.TraceID, s.Resource.Attributes); cerr != nil {
						testResult.Reason = cerr
						return testResult
					}
					if cerr := assertResourceID("span "+s.TraceID, s.Resource.Attributes); cerr != nil {
						testResult.Reason = cerr
						return testResult
					}
					found++
				}
			}
			if found > 0 {
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: no spans of %s in aws/spans", attempt, otlpService)
		}
		if attempt < maxRetries {
			log.Printf("[AKS_OTLP_Traces] %v, retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}
