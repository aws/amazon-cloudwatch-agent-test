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

	os.Exit(m.Run())
}
