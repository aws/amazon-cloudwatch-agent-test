//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// Shared constants and variables used across performance and regression tests.
const (
	agentPodFilter    = `"@resource.k8s.pod.name"=~"cloudwatch-agent.*"`
	agentNSFilter     = `"@resource.k8s.namespace.name"="amazon-cloudwatch"`
	queryRangeMinutes = 5
)

var (
	cfg    otelmetrics.TestConfig
	client *otelmetrics.OtelMetricsClient
)

// podMetricData holds the pre-fetched query results shared by both tests.
type podMetricData struct {
	CPUResults []otelmetrics.RangeResult
	MemResults []otelmetrics.RangeResult
}

var (
	sharedMetrics     *podMetricData
	sharedMetricsOnce sync.Once
	sharedMetricsErr  error
)

// fetchSharedMetrics queries CPU and memory metrics once and returns cached
// results on subsequent calls.
func fetchSharedMetrics(t *testing.T) *podMetricData {
	t.Helper()
	sharedMetricsOnce.Do(func() {
		ctx := context.Background()
		end := time.Now()
		start := end.Add(-queryRangeMinutes * time.Minute)
		step := 1 * time.Second

		// Escape the cluster name before interpolating, matching the shared
		// helper used by the other otel suites (kubeletstats/cadvisor/gpu).
		escapedCluster := otelmetrics.EscapePromQLValue(cfg.ClusterName)
		clusterFilter := fmt.Sprintf(`"@resource.k8s.cluster.name"="%s"`, escapedCluster)

		cpuQuery := fmt.Sprintf(`{"__name__"="k8s.pod.cpu.utilization", %s, %s, %s}`, agentPodFilter, agentNSFilter, clusterFilter)
		cpuResults, err := client.QueryRange(ctx, cpuQuery, start, end, step)
		if err != nil {
			sharedMetricsErr = fmt.Errorf("CPU QueryRange failed: %w", err)
			return
		}
		memQuery := fmt.Sprintf(`{"__name__"="k8s.pod.memory.working_set", %s, %s, %s}`, agentPodFilter, agentNSFilter, clusterFilter)
		memResults, err := client.QueryRange(ctx, memQuery, start, end, step)
		if err != nil {
			sharedMetricsErr = fmt.Errorf("Memory QueryRange failed: %w", err)
			return
		}
		sharedMetrics = &podMetricData{
			CPUResults: cpuResults,
			MemResults: memResults,
		}
	})
	require.NoError(t, sharedMetricsErr, "failed to fetch shared pod metrics")
	require.NotNil(t, sharedMetrics, "shared metrics are nil")
	return sharedMetrics
}

// isAllZero reports whether every value in the series is zero. An all-zero
// series means no data was collected for that pod in the window; scoring it
// would drag the stats artificially low, so callers skip these series.
func isAllZero(values []float64) bool {
	for _, v := range values {
		if v != 0 {
			return false
		}
	}
	return true
}

// calcStats computes the average and maximum from a series of data points.
// performance_test.go uses the average and regression_test.go uses the max.
func calcStats(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum, max float64
	for _, v := range values {
		sum += v
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(values))
	return avg, max
}

// summaryStat reduces a series to a single value using the named statistic
// (max, p90, p95, p99, es90, es95, or average). Driven by the "stat" field in the config.
func summaryStat(values []float64, stat string) float64 {
	if len(values) == 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(stat)) {
	case "max":
		_, max := calcStats(values)
		return max
	case "p90":
		return percentile(values, 90)
	case "p95":
		return percentile(values, 95)
	case "p99":
		return percentile(values, 99)
	case "es90":
		return expectedShortfall(values, 90)
	case "es95":
		return expectedShortfall(values, 95)
	case "average", "avg", "mean", "":
		avg, _ := calcStats(values)
		return avg
	default:
		avg, _ := calcStats(values)
		return avg
	}
}

// allStatsString formats avg/max/p95/p99/es95 for one series, for side-by-side
// comparison while deciding which statistic to gate on. Remove once chosen.
func allStatsString(values []float64) string {
	avg, max := calcStats(values)
	return fmt.Sprintf("avg=%.4f max=%.4f p95=%.4f p99=%.4f es95=%.4f",
		avg, max, percentile(values, 95), percentile(values, 99), expectedShortfall(values, 95))
}

// expectedShortfall returns the mean of the samples at or above the p-th
// percentile (CVaR / tail-conditional mean) — a smoothed view of the worst tail
// that is steadier than p95 but still moves when the tail genuinely shifts.
func expectedShortfall(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	cutoff := percentile(values, p)
	var sum float64
	var count int
	for _, v := range sorted {
		if v >= cutoff {
			sum += v
			count++
		}
	}
	if count == 0 {
		return cutoff
	}
	return sum / float64(count)
}

// percentile returns the linearly-interpolated p-th percentile (0-100) of values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// TestMain resolves the region, cluster name, and account ID into a config and
// sets up the shared metrics client before running the suite.
func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env := environment.GetEnvironmentMetaData()
	region := env.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		fmt.Fprintf(os.Stderr, "Region not set\n")
		os.Exit(1)
	}
	clusterName := env.EKSClusterName
	if clusterName == "" {
		clusterName = os.Getenv("CLUSTER_NAME")
	}
	if clusterName == "" {
		fmt.Fprintf(os.Stderr, "Cluster name not set\n")
		os.Exit(1)
	}
	if err := awsservice.ConfigureAWSClients(region); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reconfigure AWS clients for region %s: %v\n", region, err)
		os.Exit(1)
	}
	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		fmt.Fprintf(os.Stderr, "AWS config error: %v\n", err)
		os.Exit(1)
	}
	stsClient := sts.NewFromConfig(awsCfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "STS GetCallerIdentity error: %v\n", err)
		os.Exit(1)
	}
	cfg = otelmetrics.TestConfig{
		Region:         region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		ClusterName:    clusterName,
		AccountID:      *identity.Account,
		SigningService: "monitoring",
	}
	client, err = otelmetrics.NewClient(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
