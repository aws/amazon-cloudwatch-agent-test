//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package gpudriver validates NVIDIA DCGM metrics from OTel Container Insights on AKS driver-install GPU
// pools, where the chart's dcgm-exporter pod (:9400) collects them. It ports test/otel/gpu (EKS) with the
// same tests: gpuidle stands in for the idle gpu_single node group and gpuburn for gpu_multi, which runs
// multi-gpu-burn.
//
// terraform/azure/aks with test_dir=./test/azure/aks/gpudriver creates the cluster.
package gpudriver

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
	env        *environment.MetaData
	cfg        otelmetrics.TestConfig
	client     *otelmetrics.OtelMetricsClient
	queryCache *otelmetrics.QueryCache
)

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()

	region := env.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		fmt.Fprintf(os.Stderr, "Region not set\n")
		os.Exit(1)
	}

	if env.AKSClusterName == "" {
		fmt.Fprintf(os.Stderr, "aksClusterName flag is required to scope telemetry to this cluster\n")
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
		ClusterName:    env.AKSClusterName,
		AccountID:      *identity.Account,
		SigningService: "monitoring",
	}

	client, err = otelmetrics.NewClient(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client error: %v\n", err)
		os.Exit(1)
	}

	// No host-type filter: AKS nodes carry no EC2 host.type. Tests split results by node pool instead.
	queryCache = otelmetrics.NewQueryCache(client, cfg.ClusterName)

	os.Exit(m.Run())
}
