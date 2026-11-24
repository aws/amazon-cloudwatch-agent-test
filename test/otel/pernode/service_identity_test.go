//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// Fixture in resources/identity-workload.yaml.
const identityNamespace = "identity-e2e"

// TestMonitorServiceIdentity checks the service identity of ServiceMonitor and PodMonitor
// series as stored in CloudWatch. The SM/PM pipelines apply the chart-wide chain
// (_service-identity.tpl), so:
//   - service.name is the pod's resource.opentelemetry.io/service.name annotation, else
//     the workload name, not the Service name (ServiceMonitor job) or
//     <namespace>/<podmonitor> (PodMonitor job) the receiver sets;
//   - service.namespace is the pod's namespace;
//   - service.instance.id is <namespace>.<pod>.<container>, not the target address;
//   - deployment.environment.name is <cloud.platform>:<cluster>/<namespace>.
func TestMonitorServiceIdentity(t *testing.T) {
	cases := []struct {
		app, deployment, wantService string
	}{
		{"id-annotated-sm", "id-annotated", "checkout-api"},
		{"id-annotated-pm", "id-annotated", "checkout-api"},
		{"id-plain-sm", "id-plain", "id-plain"},
		{"id-plain-pm", "id-plain", "id-plain"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.app, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			defer cancel()
			promql := fmt.Sprintf(`version{"@resource.k8s.cluster.name"=%q,"@resource.k8s.namespace.name"=%q,"app"=%q}`,
				cfg.ClusterName, identityNamespace, c.app)
			var results []otelmetrics.MetricResult
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for len(results) == 0 {
				r, err := client.Query(ctx, promql)
				if err == nil {
					results = r
				}
				if len(results) > 0 {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("no %s series reached CloudWatch (last err=%v)", c.app, err)
				case <-ticker.C:
				}
			}
			for _, r := range results {
				res := r.Labels.Resource
				t.Logf("%s: service.name=%q service.namespace=%q service.instance.id=%q deployment.environment.name=%q",
					c.app, res["service.name"], res["service.namespace"], res["service.instance.id"], res["deployment.environment.name"])
				assert.Equalf(t, c.wantService, res["service.name"], "%s service.name", c.app)
				assert.Equalf(t, identityNamespace, res["service.namespace"], "%s service.namespace", c.app)
				assert.Equalf(t, c.deployment, res["k8s.deployment.name"], "%s k8s.deployment.name", c.app)
				pod, container := res["k8s.pod.name"], res["k8s.container.name"]
				require.NotEmptyf(t, pod, "%s has no k8s.pod.name", c.app)
				require.NotEmptyf(t, container, "%s has no k8s.container.name", c.app)
				assert.Equalf(t, identityNamespace+"."+pod+"."+container, res["service.instance.id"], "%s service.instance.id", c.app)
				assert.Truef(t, strings.HasSuffix(res["deployment.environment.name"], ":"+cfg.ClusterName+"/"+identityNamespace),
					"%s deployment.environment.name %q", c.app, res["deployment.environment.name"])
			}
		})
	}
}
