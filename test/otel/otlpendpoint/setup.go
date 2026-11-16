// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpendpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// EKSSuite is what the EKS suites of the OTLP endpoint share: the cluster, a PromQL client for its
// metrics, a Kubernetes client and its nodes.
type EKSSuite struct {
	Config otelmetrics.TestConfig
	Client *otelmetrics.OtelMetricsClient
	Kube   kubernetes.Interface
	// Nodes are the cluster's nodes, keyed by name. Resource attributes that come from the node (host,
	// zone, labels) are checked against them.
	Nodes map[string]corev1.Node
}

// SetupEKS builds the EKSSuite from the environment flags, which the caller has parsed, with
// AWS_REGION and CLUSTER_NAME as fallbacks. It also configures the awsservice clients.
func SetupEKS(ctx context.Context, env *environment.MetaData) (*EKSSuite, error) {
	region := env.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		return nil, errors.New("region not set")
	}
	clusterName := env.EKSClusterName
	if clusterName == "" {
		clusterName = os.Getenv("CLUSTER_NAME")
	}
	if clusterName == "" {
		return nil, errors.New("cluster name not set")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("AWS config: %w", err)
	}
	identity, err := sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("STS GetCallerIdentity: %w", err)
	}

	s := &EKSSuite{Config: otelmetrics.TestConfig{
		Region:         region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		ClusterName:    clusterName,
		AccountID:      *identity.Account,
		SigningService: "monitoring",
	}}
	if s.Client, err = otelmetrics.NewClient(ctx, s.Config); err != nil {
		return nil, fmt.Errorf("PromQL client: %w", err)
	}
	if err := awsservice.ConfigureAWSClients(region); err != nil {
		return nil, fmt.Errorf("AWS clients: %w", err)
	}
	if s.Kube, err = KubeClient(); err != nil {
		return nil, fmt.Errorf("Kubernetes client: %w", err)
	}
	list, err := s.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Kubernetes nodes: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, errors.New("Kubernetes nodes: no nodes")
	}
	s.Nodes = make(map[string]corev1.Node, len(list.Items))
	for _, node := range list.Items {
		s.Nodes[node.Name] = node
	}
	return s, nil
}
