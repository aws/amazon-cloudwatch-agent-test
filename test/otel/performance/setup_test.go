//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
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


// struct to hold the  pre-fetched query results shared by both tests.
type podMetricData struct {
	CPUResults []otelmetrics.RangeResult
	MemResults []otelmetrics.RangeResult
	QueryStart time.Time
	QueryEnd   time.Time
}
var (
	sharedMetrics     *podMetricData
	sharedMetricsOnce sync.Once
	sharedMetricsErr  error
)


// Query CPU and memory metrics and return cached results on subsequent calls.
func fetchSharedMetrics(t *testing.T) *podMetricData {
	t.Helper()
	sharedMetricsOnce.Do(func() {
		ctx := context.Background()
		end := time.Now()
		start := end.Add(-queryRangeMinutes * time.Minute)
		step := 30 * time.Second

		cpuQuery := fmt.Sprintf(`{"__name__"="k8s.pod.cpu.utilization", %s, %s}`, agentPodFilter, agentNSFilter)
		cpuResults, err := client.QueryRange(ctx, cpuQuery, start, end, step)
		if err != nil {
			sharedMetricsErr = fmt.Errorf("CPU QueryRange failed: %w", err)
			return
		}
		memQuery := fmt.Sprintf(`{"__name__"="k8s.pod.memory.working_set", %s, %s}`, agentPodFilter, agentNSFilter)
		memResults, err := client.QueryRange(ctx, memQuery, start, end, step)
		if err != nil {
			sharedMetricsErr = fmt.Errorf("Memory QueryRange failed: %w", err)
			return
		}
		sharedMetrics = &podMetricData{
			CPUResults: cpuResults,
			MemResults: memResults,
			QueryStart: start,
			QueryEnd:   end,
		}
	})
	require.NoError(t, sharedMetricsErr, "failed to fetch shared pod metrics")
	require.NotNil(t, sharedMetrics, "shared metrics are nil")
	return sharedMetrics
}


// Compute the average and maximum from a series of data points. performance_test.go uses avg and regression_test.go uses max 
func calcStats(values []float64) (avg, max float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
		if v > max {
			max = v
		}
	}
	avg = sum / float64(len(values))
	return avg, max
}


// Test setup. Get the region, cluster name, account ID etc and build them into a config. Setup the client
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
