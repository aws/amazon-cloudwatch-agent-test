//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	monitorPipeline  = "metrics/cw_k8s_ci_v0_prometheuscr"
	monitorScopeProc = "transform/cw_k8s_ci_v0_set_scope_prometheuscr"
	monitorSolution  = `"cloudwatch.solution"], "k8s-otel-prometheus-monitors"`
	monitorPipelineA = `"cloudwatch.pipeline"], "prometheus-monitors"`
	// monitorFilter drops the scrape metadata series but keeps up on the SM/PM pipeline.
	monitorFilter = "filter/cw_k8s_ci_v0_prometheuscr_scrape_metadata"
	// monitorExporter sends the SM/PM series to the regional endpoint.
	monitorExporter = "otlphttp/cw_k8s_ci_v0_prometheuscr"
	// monitorPodAttrs and monitorIdentity give the SM/PM series the chart-wide service identity.
	monitorPodAttrs = "k8sattributes/cw_k8s_ci_v0_pod"
	monitorIdentity = "transform/cw_k8s_ci_v0_service_identity"
	// otelCIEnabledEnv is set by the harness to "false" when it installs the chart with
	// otelContainerInsights.enabled=false, so the suite asserts the monitor-only shape.
	otelCIEnabledEnv = "PERNODE_OTEL_CI_ENABLED"
)

// TestPrometheusMonitorMetricsCarrySolution checks the metrics as stored in CloudWatch:
// every ServiceMonitor and PodMonitor workload series carries the scope attributes
// cloudwatch.solution=k8s-otel-prometheus-monitors and
// cloudwatch.pipeline=prometheus-monitors, so they are not labeled as Container
// Insights infrastructure.
func TestPrometheusMonitorMetricsCarrySolution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	for _, app := range workloadApps {
		results, metric := workloadResults(ctx, t, app)
		require.NotEmptyf(t, results, "no %s metrics reached CloudWatch", app)
		t.Logf("%s %s: %d series, first series instrumentation labels %v", app, metric, len(results), results[0].Labels.Instrumentation)
		for _, r := range results {
			inst := r.Labels.Instrumentation
			assert.Equalf(t, "k8s-otel-prometheus-monitors", inst["cloudwatch.solution"],
				"%s %s series cloudwatch.solution (instrumentation labels %v)", app, metric, inst)
			assert.Equalf(t, "prometheus-monitors", inst["cloudwatch.pipeline"],
				"%s %s series cloudwatch.pipeline (instrumentation labels %v)", app, metric, inst)
		}
	}
}

type otelConfig struct {
	Processors map[string]struct {
		MetricStatements []struct {
			Statements []string `yaml:"statements"`
		} `yaml:"metric_statements"`
	} `yaml:"processors"`
	Exporters map[string]struct {
		Endpoint string `yaml:"endpoint"`
	} `yaml:"exporters"`
	Service struct {
		Pipelines map[string]struct {
			Processors []string `yaml:"processors"`
			Exporters  []string `yaml:"exporters"`
		} `yaml:"pipelines"`
	} `yaml:"service"`
}

// agentOtelConfig reads and parses spec.otelConfig from an AmazonCloudWatchAgent CR.
func agentOtelConfig(t *testing.T, agentName string) otelConfig {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cr, err := dynamicClient(t).Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Get(ctx, agentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting AmazonCloudWatchAgent %s/%s", agentNamespace, agentName)
	raw, found, err := unstructured.NestedString(cr.Object, "spec", "otelConfig")
	require.NoError(t, err)
	require.Truef(t, found, "%s has no otelConfig", agentName)
	var cfg otelConfig
	require.NoErrorf(t, yaml.Unmarshal([]byte(raw), &cfg), "parsing %s otelConfig", agentName)
	return cfg
}

// TestPrometheusMonitorPipelineConfig checks the SM/PM pipeline on both agents: it labels
// its series cloudwatch.solution=k8s-otel-prometheus-monitors and
// cloudwatch.pipeline=prometheus-monitors, keeps up through its own filter, exports to
// the regional endpoint through its own exporter, finds the scraped pod and applies the
// chart-wide service identity after resourcedetection, and, when the harness turned
// otelContainerInsights off, runs without any Container Insights pipeline: the
// cluster-scraper runs it alone, and the node agent next to the OTLP endpoint's pipelines.
func TestPrometheusMonitorPipelineConfig(t *testing.T) {
	ciEnabled := os.Getenv(otelCIEnabledEnv) != "false"
	wantEndpoint := fmt.Sprintf("https://monitoring.%s.amazonaws.com:443", cfg.Region)
	for _, agent := range []string{perNodeAgentName, clusterScraperAgentName} {
		agent := agent
		t.Run(agent, func(t *testing.T) {
			cfg := agentOtelConfig(t, agent)
			pipeline, ok := cfg.Service.Pipelines[monitorPipeline]
			require.Truef(t, ok, "%s has no %s pipeline", agent, monitorPipeline)
			assert.Contains(t, pipeline.Processors, monitorScopeProc)

			var stmts []string
			for _, ms := range cfg.Processors[monitorScopeProc].MetricStatements {
				stmts = append(stmts, ms.Statements...)
			}
			joined := strings.Join(stmts, " ")
			assert.Containsf(t, joined, monitorSolution, "%s scope statements: %s", agent, joined)
			assert.Containsf(t, joined, monitorPipelineA, "%s scope statements: %s", agent, joined)

			assert.Containsf(t, pipeline.Processors, monitorFilter, "%s SM/PM processors", agent)
			assert.NotContainsf(t, pipeline.Processors, "filter/cw_k8s_ci_v0_scrape_metadata", "%s SM/PM pipeline must not drop up", agent)
			assert.Equalf(t, []string{monitorExporter}, pipeline.Exporters, "%s SM/PM exporters", agent)
			assert.Equalf(t, wantEndpoint, cfg.Exporters[monitorExporter].Endpoint,
				"%s %s endpoint", agent, monitorExporter)

			index := func(name string) int {
				for i, p := range pipeline.Processors {
					if p == name {
						return i
					}
				}
				return -1
			}
			pod, rd, id := index(monitorPodAttrs), index("resourcedetection/cw_k8s_ci_v0"), index(monitorIdentity)
			assert.Truef(t, pod >= 0 && rd > pod && id > rd, "%s SM/PM processors: want %s, then resourcedetection, then %s: %v",
				agent, monitorPodAttrs, monitorIdentity, pipeline.Processors)

			var names []string
			for name := range cfg.Service.Pipelines {
				names = append(names, name)
			}
			var ci, other []string
			for _, name := range names {
				switch {
				case name == monitorPipeline:
				case strings.Contains(name, "cw_k8s_ci_v0_"):
					ci = append(ci, name)
				default:
					other = append(other, name)
				}
			}
			if ciEnabled {
				assert.NotEmptyf(t, ci, "%s should also run its otelContainerInsights pipelines: %v", agent, names)
				return
			}
			assert.Emptyf(t, ci, "%s runs otelContainerInsights pipelines with otelContainerInsights off: %v", agent, names)
			if agent == clusterScraperAgentName {
				assert.Emptyf(t, other, "%s should run only the SM/PM pipeline: %v", agent, names)
			}
			for _, name := range other {
				assert.Containsf(t, name, "cw_k8s_otlp", "%s runs an unexpected pipeline %s", agent, name)
			}
		})
	}
}

// upSeries returns the up series CloudWatch holds for one monitor's targets.
func upSeries(ctx context.Context, namespace, app string) ([]otelmetrics.MetricResult, error) {
	promql := fmt.Sprintf(`up{"@resource.k8s.cluster.name"=%q,"@resource.k8s.namespace.name"=%q,"app"=%q}`,
		cfg.ClusterName, namespace, app)
	return client.Query(ctx, promql)
}

// TestMonitorTargetsReportUp checks that up reaches CloudWatch for ServiceMonitor and
// PodMonitor targets, so a failing target is visible as up == 0 rather than as missing
// metrics. The sm-app and pm-app targets scrape successfully (up == 1). The auth-app
// target scrapes only with its Secret's credentials, so its up is 1 with readSecrets and
// 0 without.
func TestMonitorTargetsReportUp(t *testing.T) {
	readSecrets := os.Getenv(readSecretsEnv) == "true"
	cases := []struct {
		namespace, app string
		want           float64
	}{
		{workloadNamespace, "sm-app", 1},
		{workloadNamespace, "pm-app", 1},
		{secretAuthNamespace, secretAuthApp, map[bool]float64{true: 1, false: 0}[readSecrets]},
	}
	for _, c := range cases {
		c := c
		t.Run(c.app, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			var last []otelmetrics.MetricResult
			for {
				results, err := upSeries(ctx, c.namespace, c.app)
				last = results
				if err == nil && len(results) > 0 {
					allMatch := true
					for _, r := range results {
						if r.Value != c.want {
							allMatch = false
						}
					}
					if allMatch {
						t.Logf("%s: %d up series, all %v", c.app, len(results), c.want)
						return
					}
				}
				select {
				case <-ctx.Done():
					values := make([]float64, 0, len(last))
					for _, r := range last {
						values = append(values, r.Value)
					}
					t.Fatalf("%s: want every up series == %v (readSecrets=%t), got %v (last err=%v)", c.app, c.want, readSecrets, values, err)
				case <-ticker.C:
				}
			}
		})
	}
}
