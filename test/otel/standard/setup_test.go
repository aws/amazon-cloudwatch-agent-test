//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

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

	// Auto-detect AccountID via STS
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

	hostMappings := []otelmetrics.SourceHostMapping{
		{Source: otelmetrics.SourceNodeExporter, HostTypes: clusterHostTypes},
		{Source: otelmetrics.SourceCadvisor, HostTypes: clusterHostTypes},
		{Source: otelmetrics.SourceKubeletstats, HostTypes: clusterHostTypes},
		{Source: otelmetrics.SourceControlPlane, HostTypes: nil},
		{Source: otelmetrics.SourceKubeStateMetrics, HostTypes: nil},
		{Source: otelmetrics.SourceKSMNodeScoped, HostTypes: nil},
	}

	registry := otelmetrics.NewSourceRegistry(clusterHostTypes, hostMappings,
		otelmetrics.SourceMapping{Source: otelmetrics.SourceNodeExporter, Metrics: nodeExporterMetrics},
		otelmetrics.SourceMapping{Source: otelmetrics.SourceCadvisor, Metrics: cadvisorMetrics},
		otelmetrics.SourceMapping{Source: otelmetrics.SourceKubeletstats, Metrics: kubeletstatsMetrics},
		otelmetrics.SourceMapping{Source: otelmetrics.SourceControlPlane, Metrics: controlPlaneMetrics},
		otelmetrics.SourceMapping{Source: otelmetrics.SourceKubeStateMetrics, Metrics: ksmClusterScopedMetrics},
		otelmetrics.SourceMapping{Source: otelmetrics.SourceKSMNodeScoped, Metrics: ksmNodeScopedMetrics},
	)

	queryCache = otelmetrics.NewQueryCache(client, cfg.ClusterName,
		otelmetrics.WithHostTypes(clusterHostTypes),
		otelmetrics.WithSourceRegistry(registry),
	)

	// Gate the suite on the agent's Lease-based node-metadata enrichment being
	// warm before any cached KSM node query is issued. See waitForKSMNodeEnrichment.
	waitForKSMNodeEnrichment(ctx, queryCache, 5*time.Minute, 30*time.Second)

	os.Exit(m.Run())
}

// waitForKSMNodeEnrichment polls kube_node_info until the agent's Lease-based
// node-metadata enrichment (host.id / host.image.id / cloud.availability_zone
// onto kube_node_* metrics) is warm on every node, or the timeout elapses.
//
// The enrichment is EVENTUALLY consistent: for the first few minutes after agent
// start, kube_node_* datapoints exist without the host attributes, then become
// enriched. The suite's QueryCache queries each metric name exactly once and
// caches the result, so a single early (cold) query for kube_node_info would
// poison every KSM node-bucket host-attribute assertion for the whole run.
//
// This gate runs BEFORE any cache population (all KSM tests are t.Parallel() and
// share the global queryCache, so the first cache-populating Get could come from
// any of them) and it uses GetWithFilter, which is NOT cached. Once this returns
// warm, the subsequent cached queryCache.Get("kube_node_info") fetches enriched
// data. Enrichment is monotonic (once warm, stays warm), so gating here is
// sufficient. On timeout we log and proceed rather than aborting the whole suite;
// the host-attribute assertions then surface a genuine (non-flaky) failure.
func waitForKSMNodeEnrichment(ctx context.Context, qc *otelmetrics.QueryCache, timeout, interval time.Duration) {
	deadline := time.Now().Add(timeout)
	for attempt := 1; ; attempt++ {
		results, err := qc.GetWithFilter(ctx, "kube_node_info", nil)
		warm := err == nil && len(results) > 0
		if warm {
			for _, r := range results {
				if r.Labels.Resource["host.id"] == "" || r.Labels.Resource["host.image.id"] == "" {
					warm = false
					break
				}
			}
		}
		if warm {
			fmt.Fprintf(os.Stderr, "KSM node enrichment warm after %d attempt(s): %d node(s) have non-empty host.id and host.image.id\n", attempt, len(results))
			return
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "WARNING: KSM node enrichment not warm after %s (attempt %d, err=%v, results=%d); proceeding anyway — host-attribute assertions may fail\n", timeout, attempt, err, len(results))
			return
		}
		fmt.Fprintf(os.Stderr, "KSM node enrichment not warm yet (attempt %d, err=%v, results=%d); retrying in %s\n", attempt, err, len(results), interval)
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "WARNING: context done while waiting for KSM node enrichment: %v\n", ctx.Err())
			return
		}
	}
}
