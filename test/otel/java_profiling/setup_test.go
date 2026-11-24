//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package java_profiling checks Java continuous profiling through the
// amazon-cloudwatch-observability Helm chart on EKS: the operator injects the
// ADOT Java embedded profiler's settings into Java auto-instrumented
// containers, and the profiles the CloudWatch agent exports can be queried
// back from CloudWatch. terraform/eks/daemon/otel-java-profiling installs the
// chart and the workloads in resources/.
package java_profiling

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/profilesquery"
)

// memoryExpectation is what the test expects of allocation profiling, from
// JAVA_PROFILING_EXPECT_MEMORY.
type memoryExpectation string

const (
	// memoryOn (the default): the operator must inject
	// OTEL_AWS_PROFILER_MEMORY_ENABLED=true and the allocation profile types must
	// reach CloudWatch.
	memoryOn memoryExpectation = "true"
	// memoryOff: the profiled workloads set OTEL_AWS_PROFILER_MEMORY_ENABLED=false
	// themselves, the operator must keep that value, and no allocation profile
	// type may reach CloudWatch.
	memoryOff memoryExpectation = "false"
)

var (
	region       string
	clusterName  string
	clientset    *kubernetes.Clientset
	profiles     *profilesquery.Client
	expectMemory memoryExpectation
	// otlpHTTPPort is the chart's otlp.http.port (JAVA_PROFILING_OTLP_HTTP_PORT).
	otlpHTTPPort int
)

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env := environment.GetEnvironmentMetaData()

	region = env.Region
	clusterName = env.EKSClusterName
	if clusterName == "" {
		fmt.Fprintf(os.Stderr, "-eksClusterName not set\n")
		os.Exit(1)
	}
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		fmt.Fprintf(os.Stderr, "Region not set\n")
		os.Exit(1)
	}

	switch v := memoryExpectation(os.Getenv("JAVA_PROFILING_EXPECT_MEMORY")); v {
	case "":
		expectMemory = memoryOn
	case memoryOn, memoryOff:
		expectMemory = v
	default:
		fmt.Fprintf(os.Stderr, "JAVA_PROFILING_EXPECT_MEMORY must be true or false, got %q\n", v)
		os.Exit(1)
	}

	port, err := strconv.Atoi(os.Getenv("JAVA_PROFILING_OTLP_HTTP_PORT"))
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintf(os.Stderr, "JAVA_PROFILING_OTLP_HTTP_PORT must be the chart's otlp.http.port, got %q\n", os.Getenv("JAVA_PROFILING_OTLP_HTTP_PORT"))
		os.Exit(1)
	}
	otlpHTTPPort = port

	if err := awsservice.ConfigureAWSClients(region); err != nil {
		fmt.Fprintf(os.Stderr, "AWS clients error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		fmt.Fprintf(os.Stderr, "AWS config error: %v\n", err)
		os.Exit(1)
	}
	profiles = profilesquery.NewClient(region, awsCfg.Credentials)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubeconfig error: %v\n", err)
		os.Exit(1)
	}
	clientset, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Kubernetes client error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
