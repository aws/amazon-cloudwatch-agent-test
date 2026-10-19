//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package dcgm_metrics_config

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	namespace           = "amazon-cloudwatch"
	dcgmExporterName    = "dcgm-exporter"
	managedConfigMap    = "dcgm-exporter-config-map"
	managedKey          = "dcp-metrics-included.csv"
	metricsConfigMap    = "dcgm-exporter-metrics-config"
	metricsKey          = "dcp-metrics-included.csv"
	configSHAAnnotation = "amazon-cloudwatch-agent-operator-config/sha256"
	dcgmScope           = "github.com/NVIDIA/dcgm-exporter"
	controlMetric       = "DCGM_FI_DEV_GPU_UTIL"

	configTimeout  = 2 * time.Minute
	rolloutTimeout = 5 * time.Minute
	metricTimeout  = 5 * time.Minute
	// suiteBudget caps every wait so the suite and its cleanup finish well
	// inside the terraform go test -timeout 1h, which would skip t.Cleanup.
	suiteBudget    = 38 * time.Minute
	cleanupTimeout = 5 * time.Minute
)

var dcgmExporterGVR = schema.GroupVersionResource{Group: "cloudwatch.aws.amazon.com", Version: "v1alpha1", Resource: "dcgmexporters"}

var (
	suiteDeadline time.Time
	// initialSHA is the pod template hash once the precondition passed; cleanup
	// waits for it only after this run created the ConfigMap or patched the exporter.
	initialSHA       string
	originalFrom     map[string]interface{}
	createdConfigMap bool
	patchedExporter  bool
)

// customMetrics drops DCGM_FI_DEV_FB_TOTAL from the chart's default list and
// adds DCGM_FI_DEV_SM_CLOCK, which the default list does not have.
const customMetrics = `DCGM_FI_DEV_GPU_UTIL, gauge, GPU utilization (in %).
DCGM_FI_DEV_MEM_COPY_UTIL, gauge, Memory utilization (in %).
DCGM_FI_DEV_FB_FREE, gauge, Framebuffer memory free (in MiB).
DCGM_FI_DEV_FB_USED, gauge, Framebuffer memory used (in MiB).
DCGM_FI_DEV_GPU_TEMP, gauge, GPU temperature (in C).
DCGM_FI_DEV_POWER_USAGE, gauge, Power draw (in W).
DCGM_FI_DEV_SM_CLOCK, gauge, SM clock frequency (in MHz).
DCGM_FI_DRIVER_VERSION, label, Driver Version.
`

const updatedMetrics = customMetrics + "DCGM_FI_DEV_MEM_CLOCK, gauge, Memory clock frequency (in MHz).\n"

// TestDCGMMetricsConfigFrom relies on the chart pointing the DcgmExporter at
// the well-known ConfigMap with optional: true, creates and removes that
// ConfigMap and edits the reference, and checks the operator applies, rolls
// out and falls back as documented.
// The steps share cluster state, so they run in order and stop at the first failure.
func TestDCGMMetricsConfigFrom(t *testing.T) {
	suiteDeadline = time.Now().Add(suiteBudget)
	initialSHA, originalFrom, createdConfigMap, patchedExporter = "", nil, false, false

	_, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), metricsConfigMap, metav1.GetOptions{})
	require.Error(t, err, "ConfigMap %s already exists, left over from a previous run; delete it", metricsConfigMap)
	require.True(t, apierrors.IsNotFound(err), "getting ConfigMap %s: %v", metricsConfigMap, err)

	var ref map[string]interface{}
	require.Eventually(t, func() bool {
		exporter, err := dynClient.Resource(dcgmExporterGVR).Namespace(namespace).Get(context.Background(), dcgmExporterName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		originalFrom, _, _ = unstructured.NestedMap(exporter.Object, "spec", "metricsConfigFrom")
		ref, _, _ = unstructured.NestedMap(exporter.Object, "spec", "metricsConfigFrom", "configMapKeyRef")
		return true
	}, budget(t, time.Minute, 5*time.Second), 5*time.Second, "getting DcgmExporter %s", dcgmExporterName)
	t.Cleanup(func() { restoreDefaults(t) })
	require.True(t, ref["name"] == metricsConfigMap && ref["key"] == metricsKey && ref["optional"] == true,
		"DcgmExporter %s spec.metricsConfigFrom.configMapKeyRef is %v, want name %s, key %s, optional true; the installed chart predates the well-known ConfigMap default or a previous run left it modified",
		dcgmExporterName, ref, metricsConfigMap, metricsKey)
	require.Eventually(t, func() bool { return recentSeries("DCGM_FI_DEV_FB_TOTAL") > 0 }, budget(t, 3*time.Minute, 15*time.Second), 15*time.Second,
		"DCGM_FI_DEV_FB_TOTAL from the default list is not published")
	initialSHA = takeSnapshot(t).sha

	steps := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"WellKnownConfigMapReplacesDefaults", testWellKnownConfigMapReplacesDefaults},
		{"ConfigMapUpdateRollsPods", testConfigMapUpdateRollsPods},
		{"DeletedOptionalConfigMapUsesDefaults", testDeletedOptionalConfigMapUsesDefaults},
		{"MissingRequiredConfigMapKeepsConfig", testMissingRequiredConfigMapKeepsConfig},
		{"RemovingReferenceRestoresDefaults", testRemovingReferenceRestoresDefaults},
	}
	for _, s := range steps {
		if !t.Run(s.name, s.fn) {
			t.FailNow()
		}
	}
}

func testWellKnownConfigMapReplacesDefaults(t *testing.T) {
	before := takeSnapshot(t)
	createMetricsConfigMap(t, customMetrics)

	requireConfigApplied(t, customMetrics, before)
	requireMetricPublished(t, "DCGM_FI_DEV_SM_CLOCK")
	requireMetricAbsent(t, "DCGM_FI_DEV_FB_TOTAL")
}

func testConfigMapUpdateRollsPods(t *testing.T) {
	before := takeSnapshot(t)
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), metricsConfigMap, metav1.GetOptions{})
	require.NoError(t, err, "getting ConfigMap %s", metricsConfigMap)
	cm.Data[metricsKey] = updatedMetrics
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(context.Background(), cm, metav1.UpdateOptions{})
	require.NoError(t, err, "updating ConfigMap %s", metricsConfigMap)

	requireConfigApplied(t, updatedMetrics, before)
	requireMetricPublished(t, "DCGM_FI_DEV_MEM_CLOCK")
}

func testDeletedOptionalConfigMapUsesDefaults(t *testing.T) {
	defaults := defaultMetrics(t)
	before := takeSnapshot(t)
	require.NoError(t, clientset.CoreV1().ConfigMaps(namespace).Delete(context.Background(), metricsConfigMap, metav1.DeleteOptions{}))

	requireConfigApplied(t, defaults, before)
	require.Eventually(t, func() bool { return recentSeries("DCGM_FI_DEV_FB_TOTAL") > 0 }, budget(t, metricTimeout, 30*time.Second), 30*time.Second,
		"DCGM_FI_DEV_FB_TOTAL is not published after falling back to the default list")
}

func testMissingRequiredConfigMapKeepsConfig(t *testing.T) {
	before := takeSnapshot(t)
	createMetricsConfigMap(t, customMetrics)
	requireConfigApplied(t, customMetrics, before)

	kept := takeSnapshot(t)
	patchExporter(t, `{"spec":{"metricsConfigFrom":{"configMapKeyRef":{"optional":false}}}}`)
	requireKept(t, kept, customMetrics, 30*time.Second, "the last applied metrics config or %s pods changed after setting optional: false", dcgmExporterName)
	seen, err := notFoundEvents()
	require.NoError(t, err, "listing events for %s", dcgmExporterName)
	require.NoError(t, clientset.CoreV1().ConfigMaps(namespace).Delete(context.Background(), metricsConfigMap, metav1.DeleteOptions{}))

	require.Eventually(t, func() bool {
		events, err := notFoundEvents()
		if err != nil {
			return false
		}
		for name, count := range events {
			if prev, ok := seen[name]; !ok || count > prev {
				return true
			}
		}
		return false
	}, budget(t, 2*time.Minute, 10*time.Second), 10*time.Second, "no new MetricsConfigMapNotFound warning event for %s", dcgmExporterName)

	requireKept(t, kept, customMetrics, time.Minute, "the last applied metrics config or %s pods changed after the required ConfigMap was deleted", dcgmExporterName)
}

// requireKept checks for d that the managed ConfigMap still holds csv
// and the pod template hash and pods are those in kept.
func requireKept(t *testing.T, kept snapshot, csv string, d time.Duration, msgAndArgs ...interface{}) {
	t.Helper()
	var polls atomic.Int32
	require.Never(t, func() bool {
		managed, csvOK := managedMetrics()
		hash, hashOK := podTemplateSHA()
		pods, err := listPodUIDs()
		if csvOK && hashOK && err == nil {
			polls.Add(1)
		}
		return (csvOK && managed != csv) || (hashOK && hash != kept.sha) || (err == nil && !samePods(pods, kept.pods))
	}, window(t, d), 5*time.Second, msgAndArgs...)
	require.GreaterOrEqual(t, polls.Load(), int32(2), "too few successful polls of %s over %s", dcgmExporterName, d)
}

func testRemovingReferenceRestoresDefaults(t *testing.T) {
	before := takeSnapshot(t)
	createMetricsConfigMap(t, updatedMetrics)
	requireConfigApplied(t, updatedMetrics, before)

	before = takeSnapshot(t)
	exporter := patchExporter(t, `{"spec":{"metricsConfigFrom":null}}`)
	defaults, _, _ := unstructured.NestedString(exporter.Object, "spec", "metricsConfig")
	require.NotEmpty(t, defaults, "DcgmExporter has no spec.metricsConfig")
	requireConfigApplied(t, defaults, before)
}

// snapshot is the dcgm-exporter state taken before an action that can roll the pods.
type snapshot struct {
	pods map[types.UID]struct{}
	sha  string
}

func takeSnapshot(t *testing.T) snapshot {
	t.Helper()
	hash, ok := podTemplateSHA()
	require.True(t, ok && hash != "", "%s pod template has no %s", dcgmExporterName, configSHAAnnotation)
	return snapshot{pods: podUIDs(t), sha: hash}
}

// requireConfigApplied waits for the managed ConfigMap to match csv, for the
// pod template hash to change from before and for every pod in before to be replaced.
func requireConfigApplied(t *testing.T, csv string, before snapshot) {
	t.Helper()
	require.Eventually(t, func() bool {
		metrics, ok := managedMetrics()
		hash, hashOK := podTemplateSHA()
		return ok && metrics == csv && hashOK && hash != "" && hash != before.sha
	}, budget(t, configTimeout, 5*time.Second), 5*time.Second,
		"%s/%s does not match the expected metrics config or %s pod template %s did not change", managedConfigMap, managedKey, dcgmExporterName, configSHAAnnotation)
	require.Eventually(t, func() bool {
		if !rolledOut() {
			return false
		}
		pods, err := listPodUIDs()
		if err != nil || len(pods) == 0 {
			return false
		}
		for uid := range pods {
			if _, old := before.pods[uid]; old {
				return false
			}
		}
		return true
	}, budget(t, rolloutTimeout, 10*time.Second), 10*time.Second, "%s pods were not rolled after the metrics config changed", dcgmExporterName)
}

func requireMetricPublished(t *testing.T, metricName string) {
	t.Helper()
	promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s"}`, metricName, otelmetrics.EscapePromQLValue(cfg.ClusterName))
	var results []otelmetrics.MetricResult
	require.Eventually(t, func() bool {
		var err error
		results, err = client.Query(context.Background(), promql)
		return err == nil && len(results) > 0
	}, budget(t, metricTimeout, 30*time.Second), 30*time.Second, "%s is not published", metricName)
	for _, r := range results {
		require.Equal(t, dcgmScope, r.Labels.Instrumentation["@name"], "%s instrumentation name", metricName)
	}
}

// requireMetricAbsent waits for two consecutive polls with no recent series of
// metricName while controlMetric from the same exporter is still published.
func requireMetricAbsent(t *testing.T, metricName string) {
	t.Helper()
	zeros := 0
	require.Eventually(t, func() bool {
		if recentSeries(metricName) == 0 && recentSeries(controlMetric) > 0 {
			zeros++
		} else {
			zeros = 0
		}
		return zeros >= 2
	}, budget(t, metricTimeout, 30*time.Second), 30*time.Second, "%s is still published, or %s is not, after the custom list replaced the defaults", metricName, controlMetric)
}

// recentSeries counts the series of metricName with samples in the last two
// minutes, or returns -1 when the query fails.
func recentSeries(metricName string) int {
	promql := fmt.Sprintf(`count_over_time(%s{"@resource.k8s.cluster.name"="%s"}[2m])`, metricName, otelmetrics.EscapePromQLValue(cfg.ClusterName))
	results, err := client.Query(context.Background(), promql)
	if err != nil {
		return -1
	}
	return len(results)
}

// budget caps d at the time left before suiteDeadline and fails the test when
// that leaves fewer than two ticks.
func budget(t *testing.T, d, tick time.Duration) time.Duration {
	t.Helper()
	left := time.Until(suiteDeadline)
	if left < 2*tick {
		t.Fatalf("suite budget exhausted before %s", t.Name())
	}
	if left < d {
		return left
	}
	return d
}

// window returns d for require.Never and fails the test when less than d is left.
func window(t *testing.T, d time.Duration) time.Duration {
	t.Helper()
	if time.Until(suiteDeadline) < d {
		t.Fatalf("suite budget exhausted before %s", t.Name())
	}
	return d
}

func createMetricsConfigMap(t *testing.T, csv string) {
	t.Helper()
	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: metricsConfigMap, Namespace: namespace},
		Data:       map[string]string{metricsKey: csv},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "creating ConfigMap %s", metricsConfigMap)
	createdConfigMap = true
}

func defaultMetrics(t *testing.T) string {
	t.Helper()
	exporter, err := dynClient.Resource(dcgmExporterGVR).Namespace(namespace).Get(context.Background(), dcgmExporterName, metav1.GetOptions{})
	require.NoError(t, err, "getting DcgmExporter %s", dcgmExporterName)
	defaults, _, _ := unstructured.NestedString(exporter.Object, "spec", "metricsConfig")
	require.NotEmpty(t, defaults, "DcgmExporter has no spec.metricsConfig")
	return defaults
}

func patchExporter(t *testing.T, patch string) *unstructured.Unstructured {
	t.Helper()
	patchedExporter = true
	exporter, err := dynClient.Resource(dcgmExporterGVR).Namespace(namespace).Patch(context.Background(),
		dcgmExporterName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	require.NoError(t, err, "patching DcgmExporter %s with %s", dcgmExporterName, patch)
	return exporter
}

// The getters below run inside require.Eventually and require.Never, so they
// report transient API errors with ok false instead of failing the test from
// another goroutine. A missing object, key or annotation is a value of "".

func managedMetrics() (string, bool) {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), managedConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", true
	}
	if err != nil {
		return "", false
	}
	return cm.Data[managedKey], true
}

func podTemplateSHA() (string, bool) {
	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), dcgmExporterName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", true
	}
	if err != nil {
		return "", false
	}
	return ds.Spec.Template.Annotations[configSHAAnnotation], true
}

func rolledOut() bool {
	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), dcgmExporterName, metav1.GetOptions{})
	return err == nil && ds.Status.ObservedGeneration >= ds.Generation &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled &&
		ds.Status.NumberAvailable == ds.Status.DesiredNumberScheduled
}

// listPodUIDs returns no pods when the DaemonSet is gone.
func listPodUIDs() (map[types.UID]struct{}, error) {
	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), dcgmExporterName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[types.UID]struct{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(ds.Spec.Selector),
	})
	if err != nil {
		return nil, err
	}
	uids := make(map[types.UID]struct{}, len(pods.Items))
	for _, p := range pods.Items {
		uids[p.UID] = struct{}{}
	}
	return uids, nil
}

func podUIDs(t *testing.T) map[types.UID]struct{} {
	t.Helper()
	uids, err := listPodUIDs()
	require.NoError(t, err, "listing %s pods", dcgmExporterName)
	require.NotEmpty(t, uids, "no %s pods", dcgmExporterName)
	return uids
}

func samePods(a, b map[types.UID]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for uid := range a {
		if _, ok := b[uid]; !ok {
			return false
		}
	}
	return true
}

// notFoundEvents maps the MetricsConfigMapNotFound warning events for the
// well-known ConfigMap to their counts.
func notFoundEvents() (map[string]int32, error) {
	events, err := clientset.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + dcgmExporterName + ",reason=MetricsConfigMapNotFound,type=Warning",
	})
	if err != nil {
		return nil, err
	}
	counts := map[string]int32{}
	for _, e := range events.Items {
		if !strings.Contains(e.Message, metricsConfigMap) {
			continue
		}
		count := e.Count
		if e.Series != nil && e.Series.Count > count {
			count = e.Series.Count
		}
		counts[e.Name] = count
	}
	return counts, nil
}

// restoreDefaults deletes the ConfigMap if this run created it, puts back the
// original metricsConfigFrom if this run changed it and waits for the pod
// template hash from the start of the run, so the suite can be rerun against
// the same cluster.
func restoreDefaults(t *testing.T) {
	ctx := context.Background()
	if createdConfigMap {
		if err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, metricsConfigMap, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("deleting ConfigMap %s: %v", metricsConfigMap, err)
		}
	}
	if patchedExporter {
		patch, err := json.Marshal([]map[string]interface{}{{"op": "add", "path": "/spec/metricsConfigFrom", "value": originalFrom}})
		if err == nil {
			_, err = dynClient.Resource(dcgmExporterGVR).Namespace(namespace).Patch(ctx,
				dcgmExporterName, types.JSONPatchType, patch, metav1.PatchOptions{})
		}
		if err != nil {
			t.Errorf("restoring DcgmExporter %s metricsConfigFrom: %v", dcgmExporterName, err)
		}
	}
	if initialSHA == "" || !(createdConfigMap || patchedExporter) {
		return
	}
	for deadline := time.Now().Add(cleanupTimeout); time.Now().Before(deadline); {
		time.Sleep(10 * time.Second)
		exporter, err := dynClient.Resource(dcgmExporterGVR).Namespace(namespace).Get(ctx, dcgmExporterName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		defaults, _, _ := unstructured.NestedString(exporter.Object, "spec", "metricsConfig")
		hash, hashOK := podTemplateSHA()
		if metrics, ok := managedMetrics(); ok && metrics == defaults && hashOK && hash == initialSHA && rolledOut() {
			return
		}
	}
	t.Errorf("%s did not return to pod template %s %s within %s", dcgmExporterName, configSHAAnnotation, initialSHA, cleanupTimeout)
}
