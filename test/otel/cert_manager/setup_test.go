//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package cert_manager

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

var (
	cfg    otelmetrics.TestConfig
	client *otelmetrics.OtelMetricsClient
	kube   kubernetes.Interface
)

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	suite, err := otlpendpoint.SetupEKS(context.Background(), environment.GetEnvironmentMetaData())
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup: %v\n", err)
		os.Exit(1)
	}
	cfg, client, kube = suite.Config, suite.Client, suite.Kube
	os.Exit(m.Run())
}
