//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ray

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

var (
	cfg        otelmetrics.TestConfig
	client     *otelmetrics.OtelMetricsClient
	queryCache *otelmetrics.QueryCache
)

// Instrumentation scope and pipeline set by the chart's Ray solution.
const (
	scopeRay    = "github.com/ray-project/ray"
	pipelineRay = "ray"
)

// Workloads deployed by terraform/eks/daemon/otel-ray: the RayService "fruit",
// whose RayCluster is named fruit-<suffix>, with a head and one worker group.
const (
	namespace        = "ray"
	rayService       = "fruit"
	workerGroup      = "cpu"
	headPodPattern   = rayService + "-.*-head-.*"
	workerPodPattern = rayService + "-.*-" + workerGroup + "-worker-.*"
	// The worker group sets resource.opentelemetry.io/service.name to this;
	// the head sets no annotation.
	workerServiceName = "fruit-workers"
)

// aws.service.type the chart sets on engine series (ray_vllm_*, ray_sglang_*).
const serviceTypeAIInference = "ai_inference"

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

	// The suite only uses GetWithFilter, so no source registry or host types.
	queryCache = otelmetrics.NewQueryCache(client, cfg.ClusterName)

	if err := waitForData(ctx, clusterName); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// readyTimeout bounds how long TestMain waits for the Ray data the suite needs.
const readyTimeout = 15 * time.Minute

// waitForData polls CloudWatch until every readiness query returns a value of
// at least its minimum, so the suite starts as soon as the data is there
// rather than after a fixed wait: core metrics from both Ray pods, and Serve
// and engine probe counters that have increased, which needs at least two
// samples of each.
func waitForData(ctx context.Context, clusterName string) error {
	sel := fmt.Sprintf(`"@resource.k8s.cluster.name"="%s","@resource.k8s.namespace.name"="%s"`, clusterName, namespace)
	checks := []struct {
		promql string
		min    float64
	}{
		{fmt.Sprintf(`count(count by ("@resource.k8s.pod.name") (ray_node_cpu_utilization{%s}))`, sel), 2},
		{fmt.Sprintf(`sum(increase(ray_serve_num_http_requests_total{%s}[10m]))`, sel), 1},
	}
	for _, md := range engineProbeMetrics {
		checks = append(checks, struct {
			promql string
			min    float64
		}{fmt.Sprintf(`sum(increase(%s{%s}[10m]))`, md.Name, sel), 1})
	}
	deadline := time.Now().Add(readyTimeout)
	for {
		pending := ""
		for _, c := range checks {
			results, err := client.Query(ctx, c.promql)
			if err != nil || len(results) == 0 || results[0].Value < c.min {
				pending = c.promql
				break
			}
		}
		if pending == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ray data not ready after %s: %s", readyTimeout, pending)
		}
		fmt.Printf("Waiting for Ray data: %s\n", pending)
		time.Sleep(30 * time.Second)
	}
}
