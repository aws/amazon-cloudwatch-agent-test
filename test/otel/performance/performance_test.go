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

// structs for defining the structure for storing threshold settings 
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


// Read JSON config and convert it into struct defined earlier 
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


// Query kube_node_status_allocatable for a CPU/mem. Add all the values for all nodes and then find an average. 
func getNodeAllocatable(t *testing.T, ctx context.Context, query string, resource string) (float64, int) {
	t.Helper()
	results, err := client.Query(ctx, query)
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


// Take a metric name and find the resuts from the data
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


// Return the threshold set for a given pod type
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


// Return a display label based on the pod type
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


//  1. Check if the values array is non-empty... if empty FAIL
//  2. Check that all values are >= 0 ...... if not FAIL 
//  3. Check that the average of all values is within [threshold*0.85, threshold*1.15], i.e. 15% above or below threshold %..... if not FAIL
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


// Run threshold performance test and print output
func TestPerformanceThresholds(t *testing.T) {
	// set up initial variables needed
	ctx := context.Background()
	thresholds := loadThresholds(t)
	metrics := fetchSharedMetrics(t)
	nodeAllocatable := make(map[string]float64)
	nodeCounts := make(map[string]int)

	// get the average node allocatable resource value
	for resource, query := range thresholds.NodeAllocatableQueries {
		val, count := getNodeAllocatable(t, ctx, query, resource)
		require.Greater(t, val, 0.0, "node allocatable %s must be > 0", resource)
		nodeAllocatable[resource] = val
		nodeCounts[resource] = count
	}

	t.Log("")
	t.Log("==================================================== RUNNING TestPerformanceThresholds ====================================================")
	t.Log("")

	// print node allocatable memory and CPU values and safe range
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
	
	// check each pod's resource usage against the expected threshold and report which pass or fail
	for _, metric := range thresholds.Metrics {
		results := getResultsForMetric(metrics, metric.Name)
		require.NotEmpty(t, results, "no data returned for %s — is the agent running and reporting metrics?", metric.Name)
		
		// print section header
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

			// Get the threshold for this specific pod type
			threshold, podType := getThresholdForPod(podName, metric.PodThresholds)
			if podType == "" {
				continue
			}

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

			// check if values are within bounds and log all the results
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
	}

	t.Log("")
	t.Log("DONE!")
	t.Log("")

	// Report all test failures 
	for _, msg := range failures {
		t.Errorf("  %s", msg)
	}
}
