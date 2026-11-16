//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// Fixture in resources/selector-workload.yaml. The harness sets both monitor selectors to
// select every monitor without the exclude label, so the rest of the suite is unaffected.
const (
	// monitorSelectorEnv is set by the harness to "true" when it installs the chart with
	// otelPrometheusMonitor.serviceMonitorSelector and podMonitorSelector set.
	monitorSelectorEnv = "PERNODE_MONITOR_SELECTOR"
	selectorNamespace  = "selector-e2e"
	selectorExcludeKey = "e2e.cloudwatch.aws/exclude"
	selectedSMJob      = "serviceMonitor/" + selectorNamespace + "/sel-in/0"
	excludedSMJob      = "serviceMonitor/" + selectorNamespace + "/sel-out/0"
	excludedPMJob      = "podMonitor/" + selectorNamespace + "/sel-pm-out/0"
)

// wantMonitorSelector is the selector the harness passes to the chart, as stored on the CRs.
var wantMonitorSelector = map[string]interface{}{
	"matchExpressions": []interface{}{
		map[string]interface{}{"key": selectorExcludeKey, "operator": "DoesNotExist"},
	},
}

// perNodeScrapeJobs returns the job names the per-node Target Allocator serves at
// /scrape_configs.
func perNodeScrapeJobs(t *testing.T) []string {
	t.Helper()
	u := fmt.Sprintf("https://%s.%s.svc:80/scrape_configs", perNodeTAService, agentNamespace)
	script := fmt.Sprintf("echo '<<SC'; %s %q; echo; echo 'SC>>'\n", taCurl, u)
	body := between(runTAProbe(t, k8sClientset(t), script), "<<SC", "SC>>")
	var configs map[string]json.RawMessage
	require.NoErrorf(t, json.Unmarshal([]byte(strings.TrimSpace(body)), &configs), "parsing /scrape_configs: %.300s", body)
	jobs := make([]string, 0, len(configs))
	for job := range configs {
		jobs = append(jobs, job)
	}
	sort.Strings(jobs)
	return jobs
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// selectorAppSeries returns the version series CloudWatch holds for one selector-e2e monitor.
func selectorAppSeries(ctx context.Context, app string) (int, error) {
	promql := fmt.Sprintf(`version{"@resource.k8s.cluster.name"=%q,"@resource.k8s.namespace.name"=%q,"app"=%q}`,
		cfg.ClusterName, selectorNamespace, app)
	results, err := client.Query(ctx, promql)
	return len(results), err
}

// TestMonitorSelectorFiltersMonitors checks otelPrometheusMonitor.serviceMonitorSelector and
// podMonitorSelector end to end:
//   - both AmazonCloudWatchAgent CRs carry the selector under prometheusCR;
//   - the per-node Target Allocator serves the selected ServiceMonitor and the suite's
//     other monitors, but not the ServiceMonitor or PodMonitor the selector excludes;
//   - CloudWatch receives the selected monitor's series and none from the excluded ones;
//   - the Target Allocators accept the selector (no restarts).
func TestMonitorSelectorFiltersMonitors(t *testing.T) {
	if os.Getenv(monitorSelectorEnv) != "true" {
		t.Skipf("%s is not true: the chart was installed without monitor selectors", monitorSelectorEnv)
	}

	dyn := dynamicClient(t)
	for _, agent := range []string{perNodeAgentName, clusterScraperAgentName} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cr, err := dyn.Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Get(ctx, agent, metav1.GetOptions{})
		cancel()
		require.NoErrorf(t, err, "getting AmazonCloudWatchAgent %s", agent)
		for _, field := range []string{"serviceMonitorSelector", "podMonitorSelector"} {
			got, found, err := unstructured.NestedFieldNoCopy(cr.Object, "spec", "targetAllocator", "prometheusCR", field)
			require.NoError(t, err)
			require.Truef(t, found, "%s has no prometheusCR.%s", agent, field)
			assert.Equalf(t, wantMonitorSelector, got, "%s prometheusCR.%s", agent, field)
		}
	}

	var jobs []string
	require.Eventuallyf(t, func() bool {
		jobs = perNodeScrapeJobs(t)
		return containsString(jobs, selectedSMJob)
	}, 5*time.Minute, 30*time.Second, "per-node Target Allocator never served %s; jobs: %v", selectedSMJob, jobs)
	t.Logf("per-node Target Allocator jobs: %v", jobs)
	assert.NotContainsf(t, jobs, excludedSMJob, "excluded ServiceMonitor is served")
	assert.NotContainsf(t, jobs, excludedPMJob, "excluded PodMonitor is served")
	assert.Containsf(t, jobs, secretAuthJob, "a ServiceMonitor without the exclude label in another namespace must stay selected")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		n, err := selectorAppSeries(ctx, "sel-in")
		if err == nil && n > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no sel-in series reached CloudWatch (last err=%v)", err)
		case <-ticker.C:
		}
	}
	// sel-in, sel-out and sel-pm-out scrape the same pod on the same interval, so once
	// sel-in has arrived the excluded ones would have too if they were scraped.
	for _, app := range []string{"sel-out", "sel-pm-out"} {
		qctx, qcancel := context.WithTimeout(context.Background(), time.Minute)
		n, err := selectorAppSeries(qctx, app)
		qcancel()
		require.NoErrorf(t, err, "querying %s", app)
		assert.Zerof(t, n, "%s is excluded by the selector but %d series reached CloudWatch", app, n)
	}

	pods := targetAllocatorPods(t, k8sClientset(t))
	require.NotEmpty(t, pods)
	assert.Zerof(t, totalRestarts(pods), "per-node Target Allocator restarted with the selector set")
}

// TestMonitorSelectorWebhookRejectsInvalid checks that the operator's admission webhook
// rejects an invalid monitor selector, which the CRD schema accepts, instead of letting it
// reach the Target Allocator, where it would stop all ServiceMonitor/PodMonitor scraping.
// The patches are server-side dry runs, so the CR is not changed.
func TestMonitorSelectorWebhookRejectsInvalid(t *testing.T) {
	if os.Getenv(monitorSelectorEnv) != "true" {
		t.Skipf("%s is not true: the operator under test may predate label selector support", monitorSelectorEnv)
	}
	dyn := dynamicClient(t)
	cases := []struct {
		name, patch, wantErr string
	}{
		{
			"unknown operator",
			`{"spec":{"targetAllocator":{"prometheusCR":{"serviceMonitorSelector":{"matchExpressions":[{"key":"team","operator":"Equals","values":["payments"]}]}}}}}`,
			`"Equals" is not a valid label selector operator`,
		},
		{
			"In without values",
			`{"spec":{"targetAllocator":{"prometheusCR":{"podMonitorSelector":{"matchExpressions":[{"key":"team","operator":"In"}]}}}}}`,
			"invalid spec.targetAllocator.prometheusCR.podMonitorSelector",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := dyn.Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Patch(ctx, perNodeAgentName,
				types.MergePatchType, []byte(c.patch), metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
			require.Errorf(t, err, "the webhook admitted an invalid selector (%s)", c.name)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
	// A valid selector is admitted.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := dyn.Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Patch(ctx, perNodeAgentName,
		types.MergePatchType, []byte(`{"spec":{"targetAllocator":{"prometheusCR":{"serviceMonitorSelector":{"matchLabels":{"release":"prod"}}}}}}`),
		metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
	require.NoError(t, err, "a valid selector must be admitted")
}
