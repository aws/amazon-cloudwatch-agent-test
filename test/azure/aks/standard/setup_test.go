//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package standard validates OTel Container Insights on AKS. It ports test/otel/standard (EKS), the
// kube-state-metrics, kubelet, cadvisor, node-exporter and control-plane checks, to a cluster with the
// default AKS node pool. Where the EKS tests assert AWS or EC2 attributes, the AKS versions assert the
// azure.aks equivalents, or skip with the reason.
//
// terraform/azure/aks with test_dir=./test/azure/aks/standard creates the cluster and the test workloads
// (nginx-test, ksm-test-statefulset, ksm-test-cronjob, ksm-test-job, ksm-test-replicaset).
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

	// No host-type filter and no source registry: AKS nodes carry no EC2 host.type, and the EKS suite's
	// waitForKSMNodeEnrichment gate waits for host.* attributes that AKS KSM metrics do not have.
	queryCache = otelmetrics.NewQueryCache(client, cfg.ClusterName)

	os.Exit(m.Run())
}
