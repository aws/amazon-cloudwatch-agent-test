//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package otlp tests the Helm chart's OTLP endpoint on EKS, installed with the chart defaults next to
// OTel Container Insights (terraform/eks/daemon/otel-otlp). Ordinary pods send OTLP through the
// cloudwatch-agent Service without pod attributes, over HTTP and gRPC, and the tests check what reaches
// CloudWatch: the Container Insights enrichment of the sender pod, labels included, the service
// identity, span metrics, the logs routing, traces, and that OTel Container Insights telemetry of the
// same pods keeps its own scope and gets the same service identity from the chart's shared chain.
package otlp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The sender workloads in terraform/eks/daemon/otel-otlp/main.tf.
const (
	senderNamespace = "otlp-e2e"
	senderContainer = "sender"

	// The app case: a Deployment with two replicas, one per node, and the app.kubernetes.io/name and
	// version labels. The chart's otelServiceIdentity.useAppLabels is false by default, so those labels
	// are not a source of service.name or service.version: service.name is the Deployment name.
	appDeployment   = "otlp-e2e-app"
	appNameLabel    = "otlp-e2e-app-name"
	appVersionLabel = "1.2.3"

	// containerInsightsSolution is the cloudwatch.solution of OTel Container Insights telemetry.
	containerInsightsSolution = "k8s-otel-container-insights"
)

const (
	lookback      = 15 * time.Minute
	attempts      = 10
	retryInterval = 30 * time.Second
)

// identityCases are the service identity cases, one sender workload each. Every case checks
// service.name, and some check the other attributes that tell how the agent identified the sender.
var identityCases = []struct {
	senderCase string
	// everyNode means the case has a sender on every node, so each node agent must resolve it.
	everyNode bool
	want      map[string]string
	// absent are resource attributes the case must not have.
	absent []string
}{
	// The app.kubernetes.io/name and version labels, with no service.name sent. The labels are not used
	// by default, so service.name is the Deployment name and service.version is not set.
	{"app", true, map[string]string{
		"service.name":        appDeployment,
		"k8s.deployment.name": appDeployment,
	}, []string{"service.version"}},
	// The resource.opentelemetry.io/service.* annotations win over the Deployment name.
	{"annotation", false, map[string]string{
		"service.name":    "otlp-e2e-annotated-svc",
		"service.version": "9.9.9",
	}, nil},
	// A service.name the sender set wins over the Deployment name.
	{"explicit", false, map[string]string{
		"service.name": "otlp-e2e-explicit-svc",
	}, nil},
	// unknown_service:sh counts as not set, so the Deployment name is used.
	{"unknown", false, map[string]string{
		"service.name":        "otlp-e2e-unknown",
		"k8s.deployment.name": "otlp-e2e-unknown",
	}, nil},
	// A DaemonSet pod with no identity labels: the DaemonSet name.
	{"daemonset", true, map[string]string{
		"service.name":       "otlp-e2e-ds",
		"k8s.daemonset.name": "otlp-e2e-ds",
		"k8s.workload.name":  "otlp-e2e-ds",
		"k8s.workload.type":  "DaemonSet",
	}, nil},
	// A CronJob pod with no identity labels: the CronJob name, which comes before the Job name.
	{"cronjob", false, map[string]string{
		"service.name":     "otlp-e2e-cron",
		"k8s.cronjob.name": "otlp-e2e-cron",
	}, nil},
	// A telemetrygen sender on the gRPC port 4317, which sends unknown_service:telemetrygen: the pod is
	// found from the gRPC connection and the Deployment name is used.
	{"grpc", false, map[string]string{
		"service.name":        "otlp-e2e-grpc",
		"k8s.deployment.name": "otlp-e2e-grpc",
	}, nil},
}

// metricNames is every metric the OTLP pipelines export for this cluster: the sender's counter and the
// span metrics computed from its spans.
var metricNames = []string{
	otlpendpoint.MetricName,
	"traces.span.metrics.calls",
	"traces.span.metrics.duration",
}

// TestEndpoint checks that the operator opened the default ports on the node agent's Service and host
// network, and that every post from every app sender pod, through the Service, was accepted.
func TestEndpoint(t *testing.T) {
	ctx := context.Background()
	servicePorts, found, err := otlpendpoint.ServicePorts(ctx, kube)
	require.NoError(t, err)
	require.True(t, found, "no %s/%s Service", otlpendpoint.AgentNamespace, otlpendpoint.AgentName)
	assert.Contains(t, servicePorts, int32(otlpendpoint.GRPCPort), "Service port")
	assert.Contains(t, servicePorts, int32(otlpendpoint.HTTPPort), "Service port")

	hostPorts, err := otlpendpoint.AgentHostPorts(ctx, kube)
	require.NoError(t, err)
	assert.Contains(t, hostPorts, int32(otlpendpoint.GRPCPort), "agent host port")
	assert.Contains(t, hostPorts, int32(otlpendpoint.HTTPPort), "agent host port")

	// A post can fail while an agent pod restarts, so the check is that posts succeed now: the last one,
	// and at least minAccepted in all.
	const minAccepted = 3
	for _, pod := range appPods(t) {
		statuses, err := otlpendpoint.SendStatuses(ctx, kube, senderNamespace, pod.Name)
		require.NoError(t, err)
		for _, signal := range otlpendpoint.Signals {
			posts := statuses[signal]
			require.NotEmpty(t, posts, "%s: no %s posts in the sender log", pod.Name, signal)
			accepted := 0
			for _, status := range posts {
				if status == "200" {
					accepted++
				}
			}
			assert.GreaterOrEqual(t, accepted, minAccepted, "%s: accepted %s posts of %v", pod.Name, signal, posts)
			assert.Equal(t, "200", posts[len(posts)-1], "%s: last %s post", pod.Name, signal)
		}
	}
}

// TestMetricResourceAttributes checks the enrichment of the app sender's metric from each of its pods,
// one per node. The values the agent can only get right by resolving the right pod (its name, UID,
// node and that node's instance) prove that the source IP survived the Service.
func TestMetricResourceAttributes(t *testing.T) {
	for _, pod := range appPods(t) {
		what := fmt.Sprintf("%s from %s", otlpendpoint.MetricName, pod.Name)
		results := eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
			return query(otlpendpoint.MetricName, map[string]string{"@resource.k8s.pod.name": pod.Name})
		})
		for _, r := range results {
			assertResource(t, what, expectedResource(t, pod), toAny(r.Labels.Resource))
			assert.Equal(t, otlpvalidation.CloudWatchSource, r.Labels.Instrumentation["cloudwatch.source"], what)
			assert.Equal(t, otlpendpoint.Solution, r.Labels.Instrumentation["cloudwatch.solution"], what)
			assert.Equal(t, "app", r.Labels.Datapoint[otlpendpoint.CaseAttribute], what)
		}
	}
}

// TestServiceIdentity checks service.name and its neighbours for each identity case. The case is a
// data point attribute, which no processor changes.
//
// The query keeps only telemetry the agent matched to a pod (it has k8s.namespace.name). The first
// records a new pod sends can arrive before the agent's pod cache has the pod's IP, and are then
// exported without pod attributes; the identity rules have nothing to work with for those.
func TestServiceIdentity(t *testing.T) {
	for _, c := range identityCases {
		c := c
		t.Run(c.senderCase, func(t *testing.T) {
			results := eventually(t, c.senderCase, func() ([]otelmetrics.MetricResult, error) {
				results, err := query(otlpendpoint.MetricName, map[string]string{
					otlpendpoint.CaseAttribute:     c.senderCase,
					"@resource.k8s.namespace.name": senderNamespace,
				})
				if err != nil || (c.everyNode && !fromEveryNode(results)) {
					return nil, err
				}
				return results, nil
			})
			want := map[string]string{
				"service.namespace":  senderNamespace,
				"k8s.namespace.name": senderNamespace,
			}
			for k, v := range c.want {
				want[k] = v
			}
			for _, r := range results {
				for key, value := range want {
					assert.Equal(t, value, r.Labels.Resource[key], "%s: %s", c.senderCase, key)
				}
				for _, key := range c.absent {
					assert.NotContains(t, r.Labels.Resource, key, c.senderCase)
				}
				assertNoInternal(t, c.senderCase, toAny(r.Labels.Resource))
			}
		})
	}
}

// TestSpanMetrics checks the span metrics computed from the app sender's server spans: the span's HTTP
// dimensions, and the sender's resource attributes, pod labels included.
func TestSpanMetrics(t *testing.T) {
	for _, metric := range metricNames[1:] {
		results := eventually(t, metric, func() ([]otelmetrics.MetricResult, error) {
			return query(metric, map[string]string{
				"@resource.service.name": appDeployment,
				"http.request.method":    "GET",
				"http.route":             "/e2e",
			})
		})
		for _, r := range results {
			assert.Equal(t, appDeployment, r.Labels.Resource["k8s.pod.label.app"], metric)
			assert.Equal(t, senderNamespace, r.Labels.Resource["k8s.namespace.name"], metric)
			assert.Equal(t, otlpendpoint.Solution, r.Labels.Instrumentation["cloudwatch.solution"], metric)
		}
	}
}

// TestMetricNames checks that the OTLP pipelines export exactly the expected metrics for this cluster,
// so a metric that appears or stops is caught.
func TestMetricNames(t *testing.T) {
	matcher := fmt.Sprintf(`{__name__=~".+","@resource.k8s.cluster.name"=%q,"@instrumentation.cloudwatch.solution"=%q}`,
		cfg.ClusterName, otlpendpoint.Solution)
	var got []string
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		names, err := client.MetricNames(context.Background(), matcher, time.Now().Add(-time.Hour), time.Now())
		require.NoError(t, err)
		got = names
		if len(got) >= len(metricNames) {
			break
		}
	}
	assert.ElementsMatch(t, metricNames, got)
}

// TestContainerInsights checks the OTel Container Insights telemetry of the app sender pods, from the
// node agents (cadvisor, kubeletstats) and the cluster scraper (kube-state-metrics): it keeps its own
// scope, and it gets the same service identity as the pods' OTLP telemetry, because the chart applies
// one service identity chain to both. The OTLP log records do not reach the Container Insights
// application log group.
func TestContainerInsights(t *testing.T) {
	for _, pod := range appPods(t) {
		for _, metric := range []string{"container_cpu_usage_seconds_total", "k8s.pod.cpu.usage", "kube_pod_info"} {
			what := fmt.Sprintf("%s of %s", metric, pod.Name)
			results := eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
				return query(metric, map[string]string{"@resource.k8s.pod.name": pod.Name})
			})
			for _, r := range results {
				assert.Equal(t, containerInsightsSolution, r.Labels.Instrumentation["cloudwatch.solution"], what)
				assert.Equal(t, appDeployment, r.Labels.Resource["service.name"], what)
				assert.Equal(t, senderNamespace, r.Labels.Resource["service.namespace"], what)
				assert.Equal(t, fmt.Sprintf("aws_eks:%s/%s", cfg.ClusterName, senderNamespace),
					r.Labels.Resource["deployment.environment.name"], what)
				assert.NotContains(t, r.Labels.Resource, "service.version", what)
				assertNoInternal(t, what, toAny(r.Labels.Resource))
			}
		}
	}

	// The senders' stdout reaches the Container Insights application log group, and the OTLP log
	// records do not. The absence is checked only once the group is known to receive the senders' logs.
	group := fmt.Sprintf("/aws/otel/containerinsights/%s/application", cfg.ClusterName)
	eventually(t, "sender stdout in "+group, func() ([]string, error) {
		return insightsMessages(group, "otlp-e2e-send")
	})
	otlpRecords, err := insightsMessages(group, otlpendpoint.LogBody("app"))
	require.NoError(t, err)
	assert.Empty(t, otlpRecords, "OTLP log records in %s", group)
}

// TestLogs checks the app sender's OTLP log records: the log group and stream the chart derives, the
// record, its scope and the sender's resource attributes, from both pods. It also checks the stream of
// the unknown case, which is named after the replacement service.name.
func TestLogs(t *testing.T) {
	group := otlpendpoint.LogGroup(cfg.ClusterName)
	t.Cleanup(func() { awsservice.DeleteLogGroup(group) })
	pods := map[string]corev1.Pod{}
	for _, pod := range appPods(t) {
		pods[pod.Name] = pod
	}

	stream := strings.Join([]string{senderNamespace, senderNamespace, appDeployment}, "/")
	records := eventually(t, group+" "+stream, func() ([]otlpvalidation.LogRecord, error) {
		records, err := streamRecords(group, stream)
		if err != nil || !hasPods(records, pods) {
			return nil, err
		}
		return records, nil
	})
	for _, rec := range records {
		podName := fmt.Sprint(rec.Resource.Attributes["k8s.pod.name"])
		pod, ok := pods[podName]
		if !ok {
			// A pod from before a restart: its attributes cannot be checked against a live pod.
			continue
		}
		what := "log record from " + podName
		assert.Equal(t, otlpendpoint.LogBody("app"), rec.Body, what)
		assert.Equal(t, "INFO", rec.SeverityText, what)
		assert.Equal(t, "app", fmt.Sprint(rec.Attributes[otlpendpoint.CaseAttribute]), what)
		assert.Equal(t, otlpendpoint.Solution, fmt.Sprint(rec.Scope.Attributes["cloudwatch.solution"]), what)
		assertResource(t, what, expectedResource(t, pod), rec.Resource.Attributes)
	}

	unknown := strings.Join([]string{senderNamespace, senderNamespace, "otlp-e2e-unknown"}, "/")
	eventually(t, group+" "+unknown, func() ([]otlpvalidation.LogRecord, error) {
		return streamRecords(group, unknown)
	})
}

// TestTraces checks the app sender's spans in Transaction Search: the span, its scope and the sender's
// resource attributes, from both pods.
func TestTraces(t *testing.T) {
	pods := map[string]corev1.Pod{}
	for _, pod := range appPods(t) {
		pods[pod.Name] = pod
	}
	q := fmt.Sprintf(`fields @message | filter @message like %q and @message like %q | limit 200`, cfg.ClusterName, appDeployment)
	spans := eventually(t, "spans of "+appDeployment, func() ([]otlpvalidation.SpanRecord, error) {
		messages, err := insightsMessages("aws/spans", "", q)
		if err != nil {
			return nil, err
		}
		var spans []otlpvalidation.SpanRecord
		seen := map[string]bool{}
		for _, m := range messages {
			var s otlpvalidation.SpanRecord
			if json.Unmarshal([]byte(m), &s) != nil {
				continue
			}
			spans = append(spans, s)
			seen[fmt.Sprint(s.Resource.Attributes["k8s.pod.name"])] = true
		}
		for name := range pods {
			if !seen[name] {
				return nil, nil
			}
		}
		return spans, nil
	})
	for _, s := range spans {
		pod, ok := pods[fmt.Sprint(s.Resource.Attributes["k8s.pod.name"])]
		if !ok {
			continue
		}
		attrs := map[string]string{otlpendpoint.CaseAttribute: "app", "http.request.method": "GET"}
		assert.NoError(t, otlpvalidation.AssertSpanContent(s, "GET /e2e", "SERVER", attrs, expectedResource(t, pod)))
		assertAbsent(t, "span "+s.TraceID, s.Resource.Attributes)
	}
}

// expectedResource is the resource of the app sender's telemetry from pod: the Container Insights
// enrichment of the pod, its labels and its node's labels, and the service identity.
// otlpvalidation.PresenceOnly marks the values that cannot be known from the cluster.
func expectedResource(t *testing.T, pod corev1.Pod) map[string]string {
	t.Helper()
	node, ok := nodes[pod.Spec.NodeName]
	require.True(t, ok, "pod %s is on unknown node %q", pod.Name, pod.Spec.NodeName)
	replicaSet := ""
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			replicaSet = owner.Name
		}
	}
	require.NotEmpty(t, replicaSet, "pod %s has no ReplicaSet owner", pod.Name)
	return map[string]string{
		"k8s.cluster.name":                        cfg.ClusterName,
		"k8s.namespace.name":                      senderNamespace,
		"k8s.pod.name":                            pod.Name,
		"k8s.pod.uid":                             string(pod.UID),
		"k8s.pod.start_time":                      otlpvalidation.PresenceOnly,
		"k8s.container.name":                      senderContainer,
		"k8s.node.name":                           pod.Spec.NodeName,
		"k8s.deployment.name":                     appDeployment,
		"k8s.replicaset.name":                     replicaSet,
		"k8s.workload.name":                       appDeployment,
		"k8s.workload.type":                       "Deployment",
		"cloud.provider":                          "aws",
		"cloud.platform":                          "aws_eks",
		"cloud.region":                            cfg.Region,
		"cloud.account.id":                        cfg.AccountID,
		"cloud.availability_zone":                 node.Labels["topology.kubernetes.io/zone"],
		"cloud.resource_id":                       fmt.Sprintf("arn:aws:eks:%s:%s:cluster/%s", cfg.Region, cfg.AccountID, cfg.ClusterName),
		"host.id":                                 otlpendpoint.NodeInstanceID(node),
		"host.type":                               node.Labels["node.kubernetes.io/instance-type"],
		"host.name":                               otlpvalidation.PresenceOnly,
		"host.image.id":                           otlpvalidation.PresenceOnly,
		"service.name":                            appDeployment,
		"service.namespace":                       senderNamespace,
		"service.instance.id":                     strings.Join([]string{senderNamespace, pod.Name, senderContainer}, "."),
		"deployment.environment.name":             fmt.Sprintf("aws_eks:%s/%s", cfg.ClusterName, senderNamespace),
		"k8s.pod.label.app":                       appDeployment,
		"k8s.pod.label.app.kubernetes.io/name":    appNameLabel,
		"k8s.pod.label.app.kubernetes.io/version": appVersionLabel,
		"k8s.node.label.otlp-e2e-node":            "v",
		"k8s.node.label.kubernetes.io/os":         "linux",
	}
}

// absentKeys are resource attributes that must not reach CloudWatch: labels on the chart's built-in
// exclusion list, which otelFilters applies by default.
var absentKeys = []string{
	"k8s.pod.label.pod-template-hash",
	"k8s.node.label.kubernetes.io/hostname",
	"k8s.node.label.topology.kubernetes.io/zone",
	"k8s.node.label.node.kubernetes.io/instance-type",
}

// appAbsentKeys are resource attributes the app sender's telemetry must not have: its
// app.kubernetes.io/version label is not a source of service.version while
// otelServiceIdentity.useAppLabels is false, the chart default, and the sender sets no service.version.
var appAbsentKeys = []string{"service.version"}

// absentPrefixes are resource attribute prefixes the pipelines use internally and remove before export.
// "_tmp." covers the service identity chain's _tmp.service_identity.* attributes.
var absentPrefixes = []string{"_tmp.", "aws.log."}

// assertResource checks every attribute in want and the absence of the attributes in absentKeys,
// appAbsentKeys and absentPrefixes. Attributes in neither list are allowed, so that an attribute a
// detector adds in a later agent release does not fail the suite.
func assertResource(t *testing.T, what string, want map[string]string, got map[string]any) {
	t.Helper()
	assert.NoError(t, otlpvalidation.AssertAttributes(what, want, got))
	assertAbsent(t, what, got)
}

// assertAbsent checks that the app sender's resource got has none of absentKeys and appAbsentKeys, and
// no internal attribute.
func assertAbsent(t *testing.T, what string, got map[string]any) {
	t.Helper()
	for _, key := range absentKeys {
		assert.NotContains(t, got, key, what)
	}
	for _, key := range appAbsentKeys {
		assert.NotContains(t, got, key, what)
	}
	assertNoInternal(t, what, got)
}

// assertNoInternal checks that got has no attribute with a prefix in absentPrefixes.
func assertNoInternal(t *testing.T, what string, got map[string]any) {
	t.Helper()
	for key := range got {
		for _, prefix := range absentPrefixes {
			assert.False(t, strings.HasPrefix(key, prefix), "%s: internal attribute %s", what, key)
		}
	}
}

// appPods returns the running pods of the app sender, which must be on every node.
func appPods(t *testing.T) []corev1.Pod {
	t.Helper()
	pods, err := otlpendpoint.RunningPods(context.Background(), kube, senderNamespace, "app="+appDeployment)
	require.NoError(t, err)
	onNodes := map[string]bool{}
	for _, pod := range pods {
		onNodes[pod.Spec.NodeName] = true
	}
	require.Len(t, onNodes, len(nodes), "app sender pods must run on every node")
	return pods
}

func query(metric string, labels map[string]string) ([]otelmetrics.MetricResult, error) {
	selector := fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q`, metric, cfg.ClusterName)
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		selector += fmt.Sprintf(`,%q=%q`, key, labels[key])
	}
	return client.Query(context.Background(), selector+"}")
}

func fromEveryNode(results []otelmetrics.MetricResult) bool {
	reported := map[string]bool{}
	for _, r := range results {
		reported[r.Labels.Resource["k8s.node.name"]] = true
	}
	for name := range nodes {
		if !reported[name] {
			return false
		}
	}
	return true
}

// streamRecords returns the OTLP log records of a log stream from the last lookback.
func streamRecords(group, stream string) ([]otlpvalidation.LogRecord, error) {
	since, until := time.Now().Add(-lookback), time.Now()
	events, err := awsservice.GetLogsSince(group, stream, &since, &until)
	if err != nil {
		return nil, err
	}
	records := make([]otlpvalidation.LogRecord, 0, len(events))
	for _, event := range events {
		var rec otlpvalidation.LogRecord
		if err := json.Unmarshal([]byte(aws.ToString(event.Message)), &rec); err != nil {
			return nil, fmt.Errorf("unparseable OTLP log record in %s/%s: %w", group, stream, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

func hasPods(records []otlpvalidation.LogRecord, pods map[string]corev1.Pod) bool {
	seen := map[string]bool{}
	for _, rec := range records {
		seen[fmt.Sprint(rec.Resource.Attributes["k8s.pod.name"])] = true
	}
	for name := range pods {
		if !seen[name] {
			return false
		}
	}
	return true
}

// insightsMessages runs a Logs Insights query over the last lookback and returns the @message of each
// row. With an empty query, it returns the messages that contain marker.
func insightsMessages(group, marker string, query ...string) ([]string, error) {
	q := fmt.Sprintf("fields @message | filter @message like %q | limit 1000", marker)
	if len(query) > 0 {
		q = query[0]
	}
	end := time.Now()
	rows, err := awsservice.GetLogQueryResults(group, end.Add(-lookback).Unix(), end.Unix(), q)
	if err != nil {
		return nil, err
	}
	var messages []string
	for _, row := range rows {
		for _, field := range row {
			if aws.ToString(field.Field) == "@message" {
				messages = append(messages, aws.ToString(field.Value))
			}
		}
	}
	return messages, nil
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// eventually retries fetch until it returns a non-empty result and returns it. If every attempt fails
// or is empty, it stops the test, so the checks that follow run only on data that arrived.
func eventually[T any](t *testing.T, what string, fetch func() ([]T, error)) []T {
	t.Helper()
	var result []T
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		result, err = fetch()
		if err == nil && len(result) > 0 {
			return result
		}
	}
	require.NoError(t, err, "querying %s", what)
	require.NotEmpty(t, result, "no %s after %d attempts", what, attempts)
	return result
}
