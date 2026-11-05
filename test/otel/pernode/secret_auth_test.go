//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Fixture in resources/secret-auth-workload.yaml: a ServiceMonitor whose basicAuth
// username and password come from a Secret, in front of an endpoint that rejects
// scrapes without them.
const (
	secretAuthNamespace = "secret-auth-e2e"
	secretAuthApp       = "auth-app"
	secretAuthJob       = "serviceMonitor/" + secretAuthNamespace + "/" + secretAuthApp + "/0"
	secretAuthUsername  = "e2e-scraper"
	secretAuthPassword  = "e2e-scrape-pass"
	// taRoleName is the ClusterRole every Target Allocator's service account is bound to.
	taRoleName = "cloudwatch-agent-target-allocator-role"
	// readSecretsEnv is set by the harness to the chart's otelPrometheusMonitor.readSecrets.
	readSecretsEnv = "PERNODE_READ_SECRETS"
)

type basicAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// secretAuthScrapeConfig reads the per-node Target Allocator's scrape config for the
// basicAuth job over mTLS, where the Target Allocator returns real secret values.
func secretAuthScrapeConfig(t *testing.T) (basicAuthConfig, bool) {
	t.Helper()
	u := fmt.Sprintf("https://%s.%s.svc:80/scrape_configs", perNodeTAService, agentNamespace)
	script := fmt.Sprintf("echo '<<SC'; %s %q; echo; echo 'SC>>'\n", taCurl, u)
	body := between(runTAProbe(t, k8sClientset(t), script), "<<SC", "SC>>")

	var configs map[string]struct {
		BasicAuth *basicAuthConfig `json:"basic_auth"`
	}
	require.NoErrorf(t, json.Unmarshal([]byte(strings.TrimSpace(body)), &configs), "parsing /scrape_configs: %.300s", body)
	cfg, ok := configs[secretAuthJob]
	if !ok || cfg.BasicAuth == nil {
		return basicAuthConfig{}, ok
	}
	return *cfg.BasicAuth, true
}

// taSecretsVerbs returns the verbs the Target Allocator ClusterRole grants on Secrets.
func taSecretsVerbs(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	role, err := k8sClientset(t).RbacV1().ClusterRoles().Get(ctx, taRoleName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting ClusterRole %s", taRoleName)
	var verbs []string
	for _, r := range role.Rules {
		if ruleCovers(r, "secrets") {
			verbs = append(verbs, r.Verbs...)
		}
	}
	return verbs
}

func ruleCovers(r rbacv1.PolicyRule, resource string) bool {
	for _, g := range r.APIGroups {
		if g == "" {
			for _, res := range r.Resources {
				if res == resource {
					return true
				}
			}
		}
	}
	return false
}

// TestServiceMonitorBasicAuthFromSecret checks otelPrometheusMonitor.readSecrets end to end.
//
// With readSecrets true, the Target Allocator ClusterRole grants get (and only get) on
// Secrets, the Target Allocator puts the Secret's username and password into the job's
// scrape config, and the endpoint's metrics reach CloudWatch.
//
// With readSecrets false, the ClusterRole has no Secret access and the Target
// Allocator's scrape config carries empty credentials, so the endpoint rejects the
// scrape. That is the gap readSecrets closes.
func TestServiceMonitorBasicAuthFromSecret(t *testing.T) {
	readSecrets := os.Getenv(readSecretsEnv) == "true"

	verbs := taSecretsVerbs(t)
	if readSecrets {
		assert.Equal(t, []string{"get"}, verbs, "Target Allocator ClusterRole verbs on secrets")
	} else {
		assert.Empty(t, verbs, "Target Allocator ClusterRole must not grant secrets access without readSecrets")
	}

	// The Target Allocator rebuilds its config, and re-reads Secrets, on each monitor
	// resync, so allow a few minutes for a change in RBAC to show.
	var auth basicAuthConfig
	require.Eventuallyf(t, func() bool {
		a, found := secretAuthScrapeConfig(t)
		auth = a
		if !found {
			return false
		}
		if readSecrets {
			return a.Username == secretAuthUsername && a.Password == secretAuthPassword
		}
		return a.Username == "" && a.Password == ""
	}, 7*time.Minute, 30*time.Second,
		"job %s scrape config: username=%q, password set=%t (readSecrets=%t)", secretAuthJob, auth.Username, auth.Password != "", readSecrets)

	if !readSecrets {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	// The example app exports only its version gauge until its "/" path is hit, and
	// nothing sends traffic to auth-app, so query that gauge.
	promql := fmt.Sprintf(`version{"@resource.k8s.cluster.name"=%q,"@resource.k8s.namespace.name"=%q,"app"=%q}`,
		cfg.ClusterName, secretAuthNamespace, secretAuthApp)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		results, err := client.Query(ctx, promql)
		if err == nil && len(results) > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no %s metrics reached CloudWatch: the basicAuth scrape did not succeed (last err=%v)", secretAuthApp, err)
		case <-ticker.C:
		}
	}
}
