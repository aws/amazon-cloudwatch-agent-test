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

	// The annotation case: resource.opentelemetry.io/service.name and /service.version annotations.
	annotatedDeployment = "otlp-e2e-annotated"
	annotatedService    = "otlp-e2e-annotated-svc"
	annotatedVersion    = "9.9.9"

	// The explicit case: the sender sends explicitService and the pod has a service.name annotation
	// (explicitAnnotation) and a service.version annotation (explicitVersion).
	explicitDeployment = "otlp-e2e-explicit"
	explicitService    = "otlp-e2e-explicit-svc"
	explicitAnnotation = "otlp-e2e-explicit-annotation"
	explicitVersion    = "7.7.7"

	// hostnetDaemonSet is the hostNetwork sender, which the agent cannot match to a pod.
	hostnetDaemonSet = "otlp-e2e-hostnet"

	// containerInsightsSolution is the cloudwatch.solution of OTel Container Insights telemetry.
	containerInsightsSolution = "k8s-otel-container-insights"
)

// containerInsightsMetrics are OTel Container Insights metrics of a pod from each source: cadvisor and
// kubeletstats on the node agents, kube-state-metrics on the cluster scraper.
var containerInsightsMetrics = []string{"container_cpu_usage_seconds_total", "k8s.pod.cpu.usage", "kube_pod_info"}

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
	// prefixes are resource attributes whose value must start with the given prefix.
	prefixes map[string]string
}{
	// The app.kubernetes.io/name and version labels, with no service.name sent. The labels are not used
	// by default, so service.name is the Deployment name and service.version is not set.
	{"app", true, map[string]string{
		"service.name":        appDeployment,
		"k8s.deployment.name": appDeployment,
	}, []string{"service.version"}, nil},
	// The resource.opentelemetry.io/service.* annotations win over the Deployment name.
	{"annotation", false, map[string]string{
		"service.name":    "otlp-e2e-annotated-svc",
		"service.version": "9.9.9",
	}, nil, nil},
	// A service.name the sender set wins over the Deployment name and the service.name annotation. The
	// chain is per attribute, so the service.version annotation fills the attribute the sender did not
	// set.
	{"explicit", false, map[string]string{
		"service.name":    explicitService,
		"service.version": explicitVersion,
	}, nil, nil},
	// unknown_service:sh counts as not set, so the Deployment name is used.
	{"unknown", false, map[string]string{
		"service.name":        "otlp-e2e-unknown",
		"k8s.deployment.name": "otlp-e2e-unknown",
	}, nil, nil},
	// A DaemonSet pod with no identity labels: the DaemonSet name.
	{"daemonset", true, map[string]string{
		"service.name":       "otlp-e2e-ds",
		"k8s.daemonset.name": "otlp-e2e-ds",
		"k8s.workload.name":  "otlp-e2e-ds",
		"k8s.workload.type":  "DaemonSet",
	}, nil, nil},
	// A CronJob pod with no identity labels: service.name is the CronJob name, which the identity chain
	// takes before the Job name. k8s.workload.* comes from the enrichment, which takes the Job before
	// the CronJob, so the pod's workload is its Job (otlp-e2e-cron-<schedule time>). The two names
	// differ by design today; this pins both so a change to either is seen.
	{"cronjob", false, map[string]string{
		"service.name":      "otlp-e2e-cron",
		"k8s.cronjob.name":  "otlp-e2e-cron",
		"k8s.workload.type": "Job",
	}, nil, map[string]string{
		"k8s.workload.name": "otlp-e2e-cron-",
		"k8s.job.name":      "otlp-e2e-cron-",
	}},
	// A telemetrygen sender on the gRPC port 4317, which sends unknown_service:telemetrygen: the pod is
	// found from the gRPC connection and the Deployment name is used.
	{"grpc", false, map[string]string{
		"service.name":        "otlp-e2e-grpc",
		"k8s.deployment.name": "otlp-e2e-grpc",
	}, nil, nil},
}

// metricNames is every metric the OTLP pipelines export for this cluster: the sender's counter and the
// span metrics computed from its spans.
var metricNames = []string{
	otlpendpoint.MetricName,
	"traces.span.metrics.calls",
	"traces.span.metrics.duration",
}

// TestEndpoint checks that the operator opened the default ports on the node agent's Service and host
// network, that the Service routes to the agent on the sender's node, and that every post from every
// app sender pod, through the Service, was accepted.
func TestEndpoint(t *testing.T) {
	ctx := context.Background()
	servicePorts, found, err := otlpendpoint.ServicePorts(ctx, kube)
	require.NoError(t, err)
	require.True(t, found, "no %s/%s Service", otlpendpoint.AgentNamespace, otlpendpoint.AgentName)
	assert.Contains(t, servicePorts, int32(otlpendpoint.GRPCPort), "Service port")
	assert.Contains(t, servicePorts, int32(otlpendpoint.HTTPPort), "Service port")

	// Each node agent's pod cache holds only its node's pods, so a sender is matched only if its posts
	// reach the agent on its own node.
	policy, err := otlpendpoint.ServiceInternalTrafficPolicy(ctx, kube)
	require.NoError(t, err)
	assert.Equal(t, corev1.ServiceInternalTrafficPolicyLocal, policy, "Service internalTrafficPolicy")

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

// TestNoUnmatchedTelemetry checks that the agent matches the app sender's telemetry to its pod. The
// other tests select telemetry that has pod attributes, so they would not see records the agent failed
// to match. The first records of a new pod (or after a node agent restart) can arrive before the
// agent's pod cache has the pod, so the check allows records until otlpendpoint.MatchedSince:
//   - metrics: every series of the app case, with no pod filter, has k8s.pod.name and the sender
//     namespace. An instant query still returns a series otlpendpoint.Staleness after its last sample,
//     so the check is polled until that long after MatchedSince;
//   - logs: unmatched records have no namespace and go to the otlpendpoint.UnmatchedStream stream; it
//     has no app record newer than MatchedSince.
func TestNoUnmatchedTelemetry(t *testing.T) {
	ctx := context.Background()
	since, err := otlpendpoint.MatchedSince(ctx, kube, senderNamespace, "app="+appDeployment)
	require.NoError(t, err)

	deadline := laterOf(since.Add(otlpendpoint.Staleness+time.Minute), time.Now().Add(attempts*retryInterval))
	var unmatched []map[string]string
	for {
		results, err := query(otlpendpoint.MetricName, map[string]string{otlpendpoint.CaseAttribute: "app"})
		require.NoError(t, err)
		unmatched = unmatched[:0]
		for _, r := range results {
			if r.Labels.Resource["k8s.pod.name"] == "" || r.Labels.Resource["k8s.namespace.name"] != senderNamespace {
				unmatched = append(unmatched, r.Labels.Resource)
			}
		}
		if len(results) > 0 && len(unmatched) == 0 {
			break
		}
		require.True(t, time.Now().Before(deadline), "%d app series without the sender pod at the deadline (of %d): %v",
			len(unmatched), len(results), unmatched)
		time.Sleep(retryInterval)
	}

	waitUntil(since.Add(time.Minute))
	group := otlpendpoint.LogGroup(cfg.ClusterName)
	matched := strings.Join([]string{senderNamespace, senderNamespace, appDeployment}, "/")
	eventually(t, "app records in "+matched+" since "+since.String(), func() ([]string, error) {
		return streamMessagesSince(group, matched, otlpendpoint.LogBody("app"), since)
	})
	records, err := streamMessagesSince(group, otlpendpoint.UnmatchedStream, otlpendpoint.LogBody("app"), since)
	require.NoError(t, err)
	assert.Empty(t, records, "app records in %s/%s since %s, not matched to a pod", group, otlpendpoint.UnmatchedStream, since)
}

// hostnetAbsent are attributes the hostnet sender's telemetry must not have: it is not attributed to
// any pod, including another hostNetwork pod on the node.
var hostnetAbsent = []string{"k8s.pod.name", "k8s.pod.uid", "k8s.deployment.name", "k8s.daemonset.name"}

// TestHostNetworkSender checks that the agent attributes the hostnet sender's telemetry to no pod. Its
// source IP is the node's, which k8sattributes does not match to a pod. A k8s.pod.ip pod association
// would match it to another hostNetwork pod on the node (the defect fixed in amazon-cloudwatch-agent
// #2393); the chart has none, and this is the regression guard. Its service.name is logged, not
// asserted.
func TestHostNetworkSender(t *testing.T) {
	senders, err := otlpendpoint.RunningPods(context.Background(), kube, senderNamespace, "app="+hostnetDaemonSet)
	require.NoError(t, err)
	require.NotEmpty(t, senders, "no running %s pod", hostnetDaemonSet)
	for _, pod := range senders {
		require.True(t, pod.Spec.HostNetwork, "%s is not on the host network", pod.Name)
	}

	results := eventually(t, "hostnet metrics", func() ([]otelmetrics.MetricResult, error) {
		return query(otlpendpoint.MetricName, map[string]string{otlpendpoint.CaseAttribute: "hostnet"})
	})
	for _, r := range results {
		what := fmt.Sprintf("hostnet metric from node %q", r.Labels.Resource["k8s.node.name"])
		assertNotAttributed(t, what, toAny(r.Labels.Resource))
		t.Logf("%s: service.name=%q", what, r.Labels.Resource["service.name"])
	}

	group := otlpendpoint.LogGroup(cfg.ClusterName)
	messages := eventually(t, "hostnet records in "+otlpendpoint.UnmatchedStream, func() ([]string, error) {
		return streamMessagesSince(group, otlpendpoint.UnmatchedStream, otlpendpoint.LogBody("hostnet"), time.Now().Add(-lookback))
	})
	for _, m := range messages {
		var rec otlpvalidation.LogRecord
		require.NoError(t, json.Unmarshal([]byte(m), &rec))
		what := fmt.Sprintf("hostnet record from node %v", rec.Resource.Attributes["k8s.node.name"])
		assertNotAttributed(t, what, rec.Resource.Attributes)
		t.Logf("%s: service.name=%v", what, rec.Resource.Attributes["service.name"])
	}
}

func assertNotAttributed(t *testing.T, what string, got map[string]any) {
	t.Helper()
	for _, key := range hostnetAbsent {
		assert.NotContains(t, got, key, what)
	}
	for key := range got {
		assert.False(t, strings.HasPrefix(key, "k8s.workload."), "%s: %s", what, key)
	}
	assertNoInternal(t, what, got)
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
				for key, prefix := range c.prefixes {
					assert.True(t, strings.HasPrefix(r.Labels.Resource[key], prefix), "%s: %s=%q, want prefix %q",
						c.senderCase, key, r.Labels.Resource[key], prefix)
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

// TestContainerInsights checks the OTel Container Insights telemetry of the sender pods, from the
// node agents (cadvisor, kubeletstats) and the cluster scraper (kube-state-metrics): it keeps its own
// scope, and it gets its service identity from the chart's shared chain:
//   - app pods: the same identity as their OTLP telemetry, and on the sender container's cadvisor
//     series the same service.instance.id;
//   - the annotated pod: the annotations, which only the chain reads, so this is specific to it;
//   - the explicit pod: its service.name annotation. Container Insights has no sender value, so by
//     DR-0001 its Container Insights and OTLP telemetry intentionally get different names.
//
// The app pods' stdout reaches the Container Insights application log group with the same identity,
// and the OTLP log records do not.
func TestContainerInsights(t *testing.T) {
	ctx := context.Background()
	appWant := map[string]string{
		"service.name":                appDeployment,
		"service.namespace":           senderNamespace,
		"deployment.environment.name": fmt.Sprintf("aws_eks:%s/%s", cfg.ClusterName, senderNamespace),
	}
	app := appPods(t)
	for _, pod := range app {
		assertContainerInsights(t, pod.Name, appWant, []string{"service.version"})

		what := "sender container_cpu_usage_seconds_total of " + pod.Name
		results := eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
			return query("container_cpu_usage_seconds_total", map[string]string{
				"@resource.k8s.pod.name":       pod.Name,
				"@resource.k8s.container.name": senderContainer,
			})
		})
		for _, r := range results {
			assert.Equal(t, strings.Join([]string{senderNamespace, pod.Name, senderContainer}, "."),
				r.Labels.Resource["service.instance.id"], what)
		}
	}
	for _, c := range []struct {
		deployment string
		want       map[string]string
	}{
		{annotatedDeployment, map[string]string{"service.name": annotatedService, "service.version": annotatedVersion}},
		{explicitDeployment, map[string]string{"service.name": explicitAnnotation, "service.version": explicitVersion}},
	} {
		pods, err := otlpendpoint.RunningPods(ctx, kube, senderNamespace, "app="+c.deployment)
		require.NoError(t, err)
		require.NotEmpty(t, pods, "no running %s pod", c.deployment)
		for _, pod := range pods {
			assertContainerInsights(t, pod.Name, c.want, nil)
		}
	}

	// The senders' stdout reaches the Container Insights application log group with the app pods'
	// service identity, and the OTLP log records do not. The absence is checked only once the group is
	// known to receive the senders' logs.
	group := fmt.Sprintf("/aws/otel/containerinsights/%s/application", cfg.ClusterName)
	podNames := map[string]bool{}
	for _, pod := range app {
		podNames[pod.Name] = true
	}
	stdout := eventually(t, "app sender stdout in "+group, func() ([]otlpvalidation.LogRecord, error) {
		messages, err := insightsMessages(group, "otlp-e2e-send")
		if err != nil {
			return nil, err
		}
		var records []otlpvalidation.LogRecord
		seen := map[string]bool{}
		for _, m := range messages {
			var rec otlpvalidation.LogRecord
			if json.Unmarshal([]byte(m), &rec) != nil {
				continue
			}
			if name := fmt.Sprint(rec.Resource.Attributes["k8s.pod.name"]); podNames[name] {
				records = append(records, rec)
				seen[name] = true
			}
		}
		if len(seen) < len(podNames) {
			return nil, nil
		}
		return records, nil
	})
	for _, rec := range stdout {
		what := fmt.Sprintf("application log of %v", rec.Resource.Attributes["k8s.pod.name"])
		assert.NoError(t, otlpvalidation.AssertAttributes(what, appWant, rec.Resource.Attributes))
		assert.NotContains(t, rec.Resource.Attributes, "service.version", what)
		assertNoInternal(t, what, rec.Resource.Attributes)
	}
	otlpRecords, err := insightsMessages(group, otlpendpoint.LogBody("app"))
	require.NoError(t, err)
	assert.Empty(t, otlpRecords, "OTLP log records in %s", group)
}

// assertContainerInsights checks the Container Insights metrics of pod: their scope, the attributes in
// want, the absence of the attributes in absent, and no internal attribute.
func assertContainerInsights(t *testing.T, pod string, want map[string]string, absent []string) {
	t.Helper()
	for _, metric := range containerInsightsMetrics {
		what := fmt.Sprintf("%s of %s", metric, pod)
		results := eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
			return query(metric, map[string]string{"@resource.k8s.pod.name": pod})
		})
		for _, r := range results {
			assert.Equal(t, containerInsightsSolution, r.Labels.Instrumentation["cloudwatch.solution"], what)
			for key, value := range want {
				assert.Equal(t, value, r.Labels.Resource[key], "%s: %s", what, key)
			}
			for _, key := range absent {
				assert.NotContains(t, r.Labels.Resource, key, what)
			}
			assertNoInternal(t, what, toAny(r.Labels.Resource))
		}
	}
}

// TestLogs checks the app sender's OTLP log records: the log group and stream the chart derives, the
// record, its scope and the sender's resource attributes, from both pods. It also checks the streams of
// the unknown case, which is named after the replacement service.name, and of the explicit case, which
// is named after the service.name the sender set.
func TestLogs(t *testing.T) {
	group := otlpendpoint.LogGroup(cfg.ClusterName)
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

	for _, name := range []string{"otlp-e2e-unknown", explicitService} {
		other := strings.Join([]string{senderNamespace, senderNamespace, name}, "/")
		eventually(t, group+" "+other, func() ([]otlpvalidation.LogRecord, error) {
			return streamRecords(group, other)
		})
	}
}

// TestTraces checks the app sender's spans in Transaction Search: the span, its scope and the sender's
// resource attributes, from both pods.
func TestTraces(t *testing.T) {
	otlpendpoint.SkipWithoutTransactionSearch(t)
	pods := map[string]corev1.Pod{}
	for _, pod := range appPods(t) {
		pods[pod.Name] = pod
	}
	q := fmt.Sprintf(`fields @message | filter @message like %q and @message like %q | limit 1000`, cfg.ClusterName, appDeployment)
	spans := eventually(t, "spans of "+appDeployment, func() ([]otlpvalidation.SpanRecord, error) {
		messages, err := insightsMessages(otlpendpoint.SpansLogGroup, "", q)
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

// streamMessagesSince returns the @message of the records in stream of group since since whose
// message contains marker, from Logs Insights. A stream with no records returns none.
func streamMessagesSince(group, stream, marker string, since time.Time) ([]string, error) {
	q := fmt.Sprintf("fields @message | filter @logStream = %q and @message like %q | limit 1000", stream, marker)
	rows, err := awsservice.GetLogQueryResults(group, since.Unix(), time.Now().Unix(), q)
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

// waitUntil sleeps until t.
func waitUntil(t time.Time) {
	if d := time.Until(t); d > 0 {
		time.Sleep(d)
	}
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
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
