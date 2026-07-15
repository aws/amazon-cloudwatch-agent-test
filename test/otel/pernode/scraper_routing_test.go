//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Annotation-based ServiceMonitor/PodMonitor routing across CloudWatch agents.
//
// By default a monitor is scraped by the per-node targetAgent. Adding the annotation
//   cloudwatch.aws/scraper: cluster-scraper
// on the ServiceMonitor/PodMonitor CR routes it to the central cluster-scraper agent instead.
// The chart wires this by setting spec.targetAllocator.prometheusCR.scraperRole=cluster-scraper on
// the cluster-scraper AmazonCloudWatchAgent CR (and leaving it empty on the per-node agent). Each
// agent's Target Allocator then filters discovered monitors by that annotation + role, so every
// monitor is owned by exactly one agent (no double-scrape, no gap).
//
// These assertions are deterministic and checkable out-of-cluster (CR spec + TA Deployment health);
// the per-monitor claim is exercised by the operator unit test TestAnnotationRoleMatches.

const (
	perNodeAgentName        = "cloudwatch-agent"
	clusterScraperAgentName = "cloudwatch-agent-cluster-scraper"
	// clusterScraperTADeploymentName is the TA the operator creates for the cluster-scraper agent.
	clusterScraperTADeploymentName = "cloudwatch-agent-cluster-scraper-target-allocator"
	clusterScraperRoleValue        = "cluster-scraper"
)

var gvrAmazonCloudWatchAgent = schema.GroupVersionResource{
	Group:    "cloudwatch.aws.amazon.com",
	Version:  "v1alpha1",
	Resource: "amazoncloudwatchagents",
}

// scraperRoleOf reads spec.targetAllocator.prometheusCR.scraperRole from an AmazonCloudWatchAgent CR.
func scraperRoleOf(t *testing.T, agentName string) string {
	t.Helper()
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	require.NoError(t, err, "building kubeconfig")
	dyn, err := dynamic.NewForConfig(restConfig)
	require.NoError(t, err, "creating dynamic client")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cr, err := dyn.Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Get(ctx, agentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting AmazonCloudWatchAgent %s/%s", agentNamespace, agentName)

	role, found, err := unstructuredNestedString(cr.Object, "spec", "targetAllocator", "prometheusCR", "scraperRole")
	require.NoError(t, err, "reading scraperRole")
	if !found {
		return ""
	}
	return role
}

func unstructuredNestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return "", false, nil
		}
		if i == len(fields)-1 {
			s, ok := v.(string)
			if !ok {
				return "", false, nil
			}
			return s, true, nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		cur = next
	}
	return "", false, nil
}

// TestScraperRoleWiring asserts the chart wires annotation-based routing: the cluster-scraper agent
// carries scraperRole=cluster-scraper (claims only annotated monitors) and the per-node agent
// carries no scraperRole (default role: claims only unannotated monitors). The two roles are
// complementary, giving exactly-one ownership.
func TestScraperRoleWiring(t *testing.T) {
	assert.Equal(t, clusterScraperRoleValue, scraperRoleOf(t, clusterScraperAgentName),
		"cluster-scraper agent should have scraperRole=cluster-scraper so it scrapes only annotated monitors")
	assert.Empty(t, scraperRoleOf(t, perNodeAgentName),
		"per-node agent should have no scraperRole (default role) so it scrapes only unannotated monitors")
}

// TestClusterScraperTargetAllocatorHealthy verifies the operator built a Target Allocator for the
// cluster-scraper agent (the target of annotation routing) and that it is Available and not
// crashlooping -- i.e. the annotation-routing path is live end to end at the infrastructure level.
func TestClusterScraperTargetAllocatorHealthy(t *testing.T) {
	clientset := k8sClientset(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(ctx, clusterScraperTADeploymentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting cluster-scraper Target Allocator %s/%s", agentNamespace, clusterScraperTADeploymentName)

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	require.Positive(t, desired, "cluster-scraper Target Allocator has 0 desired replicas")
	require.Equalf(t, desired, dep.Status.ReadyReplicas,
		"cluster-scraper Target Allocator not fully ready: ready=%d desired=%d", dep.Status.ReadyReplicas, desired)
	assert.True(t, deploymentAvailable(dep), "cluster-scraper Target Allocator Deployment Available condition is not True")

	pods, err := clientset.CoreV1().Pods(agentNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + clusterScraperTADeploymentName,
	})
	require.NoError(t, err, "listing cluster-scraper Target Allocator pods")
	require.NotEmpty(t, pods.Items, "no cluster-scraper Target Allocator pods found")
	for _, p := range pods.Items {
		assert.Equalf(t, corev1.PodRunning, p.Status.Phase, "cluster-scraper TA pod %s phase=%s", p.Name, p.Status.Phase)
		for _, cs := range p.Status.ContainerStatuses {
			assert.Truef(t, cs.Ready, "cluster-scraper TA pod %s container %s not Ready", p.Name, cs.Name)
			if cs.State.Waiting != nil {
				assert.NotEqualf(t, "CrashLoopBackOff", cs.State.Waiting.Reason,
					"cluster-scraper TA pod %s container %s in CrashLoopBackOff", p.Name, cs.Name)
			}
		}
	}
}
