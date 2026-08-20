//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// performanceThresholds defines the structure for the stored threshold settings.
type performanceThresholds struct {
	ErrorBound             float64           `json:"error_bound"`
	Metrics                []metricThreshold `json:"metrics"`
	NodeAllocatableQueries map[string]string `json:"node_allocatable_queries"`
}

type metricThreshold struct {
	Name          string         `json:"name"`
	Unit          string         `json:"unit"`
	Stat          string         `json:"stat"`
	PodThresholds []podThreshold `json:"pod_thresholds"`
}

type podThreshold struct {
	PodFilter   string  `json:"pod_filter"`
	Threshold   float64 `json:"threshold"`
	Description string  `json:"description"`
}

// loadThresholds reads the JSON config and unmarshals it into performanceThresholds.
func loadThresholds(t *testing.T) performanceThresholds {
	t.Helper()
	configPath := filepath.Join("resources", "performance_thresholds.json")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err, "failed to read performance thresholds config at %s", configPath)
	var thresholds performanceThresholds
	err = json.Unmarshal(data, &thresholds)
	require.NoError(t, err, "failed to parse performance thresholds JSON")
	require.NotEmpty(t, thresholds.Metrics, "no metrics defined in thresholds config")
	return thresholds
}

// getNodeAllocatable queries the node-allocatable metric for a CPU/mem resource,
// sums the values across all nodes, and returns the average and node count.
func getNodeAllocatable(t *testing.T, metricName string, resource string) (float64, int) {
	t.Helper()
	// Build the query scoped to this cluster, mirroring how the pod queries are
	// constructed. The monitoring endpoint is account/region-wide, so without the
	// cluster predicate we'd average node allocatable across other clusters' nodes
	// and skew the percent-of-node denominator the thresholds are calibrated to.
	query := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s", resource="%s"}`,
		metricName, otelmetrics.EscapePromQLValue(cfg.ClusterName), resource)
	results, err := client.Query(context.Background(), query)
	require.NoError(t, err, "failed to query node allocatable for resource=%s", resource)
	require.NotEmpty(t, results, "no kube_node_status_allocatable data for resource=%s — are KSM metrics being collected?", resource)
	var sum float64
	for _, r := range results {
		require.GreaterOrEqual(t, r.Value, 0.0,
			"node allocatable %s has negative value %.4f — data corruption?", resource, r.Value)
		sum += r.Value
	}
	avg := sum / float64(len(results))
	return avg, len(results)
}

// getResultsForMetric returns the fetched series for the given metric name.
func getResultsForMetric(metrics *podMetricData, metricName string) []otelmetrics.RangeResult {
	switch metricName {
	case "k8s.pod.cpu.utilization":
		return metrics.CPUResults
	case "k8s.pod.memory.working_set":
		return metrics.MemResults
	default:
		return nil
	}
}

// getThresholdForPod returns the threshold and pod-class label for a given pod.
func getThresholdForPod(podName string, podThresholds []podThreshold) (float64, string) {
	for _, pt := range podThresholds {
		switch pt.PodFilter {
		case "daemonset":
			if isDaemonSetPod(podName) {
				return pt.Threshold, pt.PodFilter
			}
		case "scraper":
			if !isDaemonSetPod(podName) {
				return pt.Threshold, pt.PodFilter
			}
		}
	}
	return 0, ""
}

// podTypeLabel returns a display label for the given pod type.
func podTypeLabel(podType string) string {
	switch podType {
	case "daemonset":
		return "DaemonSet pod"
	case "scraper":
		return "Scraper pod"
	default:
		return "Unknown pod"
	}
}

// isAllValuesWithinBound returns the average of values and an error if the set
// is empty, contains a negative value, or the average falls outside the
// threshold band [threshold*(1-errorBound), threshold*(1+errorBound)].
func isAllValuesWithinBound(values []float64, threshold float64, errorBound float64) (float64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("no values found")
	}
	totalSum := 0.0
	for _, value := range values {
		if value < 0 && threshold >= 0 {
			return 0, fmt.Errorf("values are not all greater than or equal to zero")
		}
		totalSum += value
	}
	avg := totalSum / float64(len(values))
	upperBound := threshold * (1 + errorBound)
	lowerBound := threshold * (1 - errorBound)
	if threshold > 0 && (avg > upperBound || avg < lowerBound) {
		return avg, fmt.Errorf("average value %f is not within bound [%f, %f]", avg, lowerBound, upperBound)
	}
	return avg, nil
}

// TestPerformanceThresholds checks each agent pod's resource usage against the
// calibrated per-node thresholds and reports pass/fail per pod class.
func TestPerformanceThresholds(t *testing.T) {
	// Set up initial variables.
	thresholds := loadThresholds(t)
	metrics := fetchSharedMetrics(t)
	nodeAllocatable := make(map[string]float64)
	nodeCounts := make(map[string]int)

	// Get the average node allocatable resource value.
	for resource, metricName := range thresholds.NodeAllocatableQueries {
		val, count := getNodeAllocatable(t, metricName, resource)
		require.Greater(t, val, 0.0, "node allocatable %s must be > 0", resource)
		nodeAllocatable[resource] = val
		nodeCounts[resource] = count
	}

	t.Log("")
	t.Log("==================================================== RUNNING TestPerformanceThresholds ====================================================")
	t.Log("")

	// Print node allocatable memory and CPU values and the safe range.
	for _, pt := range thresholds.Metrics[0].PodThresholds {
		label := podTypeLabel(pt.PodFilter)
		t.Logf("  (%s): Node allocatable CPU: %.4f cores (avg across %d nodes)",
			label, nodeAllocatable["cpu"], nodeCounts["cpu"])
		t.Logf("  (%s): Node allocatable memory: %.2f MB (avg across %d nodes)",
			label, nodeAllocatable["memory"]/(1024*1024), nodeCounts["memory"])
		for _, m := range thresholds.Metrics {
			for _, mpt := range m.PodThresholds {
				if mpt.PodFilter == pt.PodFilter {
					lower := mpt.Threshold * (1 - thresholds.ErrorBound)
					upper := mpt.Threshold * (1 + thresholds.ErrorBound)
					switch m.Name {
					case "k8s.pod.cpu.utilization":
						t.Logf("  (%s): Node CPU safe range: ±%.0f%% of %.2f%% of Node allocatable CPU [%.4f%%, %.4f%%]",
							label, thresholds.ErrorBound*100, mpt.Threshold, lower, upper)
					case "k8s.pod.memory.working_set":
						t.Logf("  (%s): Node memory safe range: ±%.0f%% of %.2f%% of Node allocatable memory [%.4f%%, %.4f%%]",
							label, thresholds.ErrorBound*100, mpt.Threshold, lower, upper)
					}
				}
			}
		}
		t.Log("")
	}

	var failures []string

	// Check each pod's resource usage against the expected threshold and report which pass or fail.
	for _, metric := range thresholds.Metrics {
		results := getResultsForMetric(metrics, metric.Name)
		require.NotEmpty(t, results, "no data returned for %s — is the agent running and reporting metrics?", metric.Name)

		// Track which pod classes the config expects vs. which we actually
		// observe, so a whole class going missing fails instead of silently
		// covering only half the thresholds.
		expectedClasses := make(map[string]bool)
		for _, pt := range metric.PodThresholds {
			expectedClasses[pt.PodFilter] = false
		}

		// Print section header.
		switch metric.Name {
		case "k8s.pod.cpu.utilization":
			t.Log("------------------------------ CPU Utilization (% of node) ------------------------------")
			t.Log("")
		case "k8s.pod.memory.working_set":
			t.Log("------------------------------ Memory Working Set (% of node) ------------------------------")
			t.Log("")
		}

		for _, series := range results {
			podName := series.Labels.Resource["k8s.pod.name"]
			require.NotEmpty(t, podName, "series is missing the k8s.pod.name resource label")

			// Get the threshold for this specific pod type
			threshold, podType := getThresholdForPod(podName, metric.PodThresholds)
			if podType == "" {
				continue
			}
			expectedClasses[podType] = true

			var denominator float64
			var resourceLabel string
			switch metric.Name {
			case "k8s.pod.cpu.utilization":
				denominator = nodeAllocatable["cpu"]
				resourceLabel = "node CPU"
			case "k8s.pod.memory.working_set":
				denominator = nodeAllocatable["memory"]
				resourceLabel = "node memory"
			default:
				t.Fatalf("unknown metric %s", metric.Name)
			}

			// Convert raw values to percentage-of-node
			pctValues := make([]float64, len(series.Values))
			for i, val := range series.Values {
				pctValues[i] = (val / denominator) * 100
			}

			// Check if values are within bounds and log all the results.
			label := podTypeLabel(podType)
			avg, err := isAllValuesWithinBound(pctValues, threshold, thresholds.ErrorBound)

			lowerBound := threshold * (1 - thresholds.ErrorBound)
			upperBound := threshold * (1 + thresholds.ErrorBound)

			t.Logf("  %s (%s): Using %.4f%% of %s", label, podName, avg, resourceLabel)

			if err != nil {
				if avg < lowerBound {
					t.Logf("    %.4f%% %sBELOW%s range [%.4f%%, %.4f%%], Threshold Test: %sFAIL%s",
						avg, colorRed, colorReset, lowerBound, upperBound, colorRed, colorReset)
				} else if avg > upperBound {
					t.Logf("    %.4f%% %sABOVE%s range [%.4f%%, %.4f%%], Threshold Test: %sFAIL%s",
						avg, colorRed, colorReset, lowerBound, upperBound, colorRed, colorReset)
				} else {
					t.Logf("    %s%s%s", colorRed, err.Error(), colorReset)
				}
				failures = append(failures, fmt.Sprintf(
					"%s (%s) [%s]: %s", label, podName, metric.Name, err.Error()))
			} else {
				t.Logf("    %.4f%% %sWITHIN%s range [%.4f%%, %.4f%%], Threshold Test: %sPASS%s",
					avg, colorGreen, colorReset, lowerBound, upperBound, colorGreen, colorReset)
			}
			t.Log("")
		}

		for class, seen := range expectedClasses {
			require.True(t, seen, "no %s pod series observed for %s — expected both pod classes to be present", class, metric.Name)
		}
	}

	t.Log("")
	t.Log("DONE!")
	t.Log("")

	// Report all test failures
	for _, msg := range failures {
		t.Errorf("  %s", msg)
	}
}
