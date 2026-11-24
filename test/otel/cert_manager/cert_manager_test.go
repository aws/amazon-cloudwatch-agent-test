//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package cert_manager tests the Helm chart with agent.certManager.enabled and
// admissionWebhooks.certManager.enabled (terraform/eks/daemon/otel-cert-manager), so cert-manager
// issues the TLS certificates that the agent, the cluster scraper and their scrape targets use.
package cert_manager

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	namespace = "amazon-cloudwatch"

	agentCertSecret         = "amazon-cloudwatch-observability-agent-cert"
	agentServerCertSecret   = "amazon-cloudwatch-observability-agent-server-cert"
	agentClientCertSecret   = "amazon-cloudwatch-observability-agent-client-cert"
	agentTAClientCertSecret = "amazon-cloudwatch-observability-agent-ta-client-cert"

	attempts      = 20
	retryInterval = 30 * time.Second
)

// TestAgentCertificatesShareOneCA checks that every agent certificate chains to the ca.crt of every
// other agent certificate Secret. The chart's TLS clients trust the ca.crt of a different Secret than
// the one holding the peer's tls.crt, so this is what agent.autoGenerateCert guarantees and what
// cert-manager mode must reproduce. It fails fast with the reason the metric checks below would fail.
func TestAgentCertificatesShareOneCA(t *testing.T) {
	ctx := context.Background()
	secrets := []string{agentCertSecret, agentServerCertSecret, agentClientCertSecret, agentTAClientCertSecret}
	for _, leaf := range secrets {
		for _, trust := range secrets {
			_, err := secretCert(ctx, t, leaf).Verify(x509.VerifyOptions{
				Roots:     secretPool(ctx, t, trust),
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			})
			assert.NoError(t, err, "%s tls.crt does not chain to %s ca.crt", leaf, trust)
		}
	}
}

// TestTLSScrapedMetricsPublished checks that metrics the agent can only collect over TLS with the
// cert-manager certificates reach CloudWatch: node-exporter is scraped by the node agent, and
// kube-state-metrics by the cluster scraper.
func TestTLSScrapedMetricsPublished(t *testing.T) {
	for _, tc := range []struct{ source, metric string }{
		{"node-exporter", "node_load1"},
		{"kube-state-metrics", "kube_node_info"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			query := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s"}`,
				tc.metric, otelmetrics.EscapePromQLValue(cfg.ClusterName))
			var results []otelmetrics.MetricResult
			var err error
			for i := 0; i < attempts; i++ {
				results, err = client.Query(context.Background(), query)
				if err == nil && len(results) > 0 {
					return
				}
				time.Sleep(retryInterval)
			}
			assert.NoError(t, err)
			assert.NotEmpty(t, results, "%s: no %s for cluster %s after %s; the TLS scrape likely failed, see the agent logs",
				tc.source, tc.metric, cfg.ClusterName, time.Duration(attempts)*retryInterval)
		})
	}
}

// TestNoCertificateErrorsInAgentLogs checks that neither agent logged a certificate verification
// error, which names the failing scrape directly when TestTLSScrapedMetricsPublished fails.
func TestNoCertificateErrorsInAgentLogs(t *testing.T) {
	ctx := context.Background()
	for _, selector := range []string{
		"app.kubernetes.io/name=cloudwatch-agent",
		"app.kubernetes.io/name=cloudwatch-agent-cluster-scraper",
	} {
		pods, err := kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		require.NoError(t, err)
		require.NotEmpty(t, pods.Items, "no pods match %s", selector)
		for _, pod := range pods.Items {
			raw, err := kube.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Do(ctx).Raw()
			require.NoError(t, err)
			for _, line := range strings.Split(string(raw), "\n") {
				assert.NotContains(t, line, "x509:", "pod %s", pod.Name)
			}
		}
	}
}

func secretData(ctx context.Context, t *testing.T, secret, key string) []byte {
	t.Helper()
	s, err := kube.CoreV1().Secrets(namespace).Get(ctx, secret, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, s.Data[key], "secret %s has no %s", secret, key)
	return s.Data[key]
}

func secretCert(ctx context.Context, t *testing.T, secret string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(secretData(ctx, t, secret, "tls.crt"))
	require.NotNil(t, block, "secret %s tls.crt is not PEM", secret)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

func secretPool(ctx context.Context, t *testing.T, secret string) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(secretData(ctx, t, secret, "ca.crt")), "secret %s ca.crt is not PEM", secret)
	return pool
}
