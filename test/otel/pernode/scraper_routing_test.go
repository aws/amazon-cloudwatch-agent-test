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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

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
// the per-monitor routing decision (annotated monitor filtered into the scrape jobs) is exercised by
// the operator test TestLoadConfigScraperRouting.

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
func scraperRoleOf(t *testing.T, dyn dynamic.Interface, agentName string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cr, err := dyn.Resource(gvrAmazonCloudWatchAgent).Namespace(agentNamespace).Get(ctx, agentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting AmazonCloudWatchAgent %s/%s", agentNamespace, agentName)

	role, found, err := unstructured.NestedString(cr.Object, "spec", "targetAllocator", "prometheusCR", "scraperRole")
	require.NoError(t, err, "reading scraperRole")
	if !found {
		return ""
	}
	return role
}

// TestScraperRoleWiring asserts the chart wires annotation-based routing: the cluster-scraper agent
// carries scraperRole=cluster-scraper (claims only annotated monitors) and the per-node agent
// carries no scraperRole (default role: claims only unannotated monitors). The two roles are
// complementary, giving exactly-one ownership.
func TestScraperRoleWiring(t *testing.T) {
	dyn := dynamicClient(t)
	assert.Equal(t, clusterScraperRoleValue, scraperRoleOf(t, dyn, clusterScraperAgentName),
		"cluster-scraper agent should have scraperRole=cluster-scraper so it scrapes only annotated monitors")
	assert.Empty(t, scraperRoleOf(t, dyn, perNodeAgentName),
		"per-node agent should have no scraperRole (default role) so it scrapes only unannotated monitors")
}

// TestClusterScraperTargetAllocatorHealthy verifies the operator built a Target Allocator for the
// cluster-scraper agent (the target of annotation routing) and that it is Available and not
// crashlooping -- i.e. the annotation-routing path is live end to end at the infrastructure level.
func TestClusterScraperTargetAllocatorHealthy(t *testing.T) {
	clientset := k8sClientset(t)

	depCtx, depCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer depCancel()
	dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(depCtx, clusterScraperTADeploymentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting cluster-scraper Target Allocator %s/%s", agentNamespace, clusterScraperTADeploymentName)

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	require.Positive(t, desired, "cluster-scraper Target Allocator has 0 desired replicas")
	require.Equalf(t, desired, dep.Status.ReadyReplicas,
		"cluster-scraper Target Allocator not fully ready: ready=%d desired=%d", dep.Status.ReadyReplicas, desired)
	assert.True(t, deploymentAvailable(dep), "cluster-scraper Target Allocator Deployment Available condition is not True")

	podCtx, podCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer podCancel()
	pods, err := clientset.CoreV1().Pods(agentNamespace).List(podCtx, metav1.ListOptions{
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


const (
	perNodeTAService        = "cloudwatch-agent-target-allocator-service"
	clusterScraperTAService = "cloudwatch-agent-cluster-scraper-target-allocator-service"
	// Job-name substrings from resources/routing-workload.yaml.
	routedJobMarker  = "routed-exporter-cluster-scraped"
	defaultJobMarker = "default-exporter-node-scraped"
)

// taJobsPartition runs an in-cluster probe pod that curls each Target Allocator's
// /jobs endpoint (HTTPS, server-cert skipped like the manual runbook -- no client
// cert required for /jobs) and returns the raw job listing each TA reports. It
// retries inside the pod to absorb TA discovery lag.
func taJobsPartition(t *testing.T, clientset *kubernetes.Clientset) (clusterScraperJobs, perNodeJobs string) {
	t.Helper()
	csURL := fmt.Sprintf("https://%s.%s.svc:80/jobs", clusterScraperTAService, agentNamespace)
	pnURL := fmt.Sprintf("https://%s.%s.svc:80/jobs", perNodeTAService, agentNamespace)
	script := fmt.Sprintf(`
for i in $(seq 1 12); do
  cs=$(curl -sk --max-time 10 %q || true)
  pn=$(curl -sk --max-time 10 %q || true)
  if echo "$cs" | grep -q %q && echo "$pn" | grep -q %q; then break; fi
  sleep 15
done
echo "===CS==="; echo "$cs"; echo "===PN==="; echo "$pn"; echo "===END==="
`, csURL, pnURL, routedJobMarker, defaultJobMarker)

	pod, err := clientset.CoreV1().Pods(agentNamespace).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "ta-jobs-probe-", Namespace: agentNamespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "probe",
				Image:   "curlimages/curl:8.10.1",
				Command: []string{"sh", "-c", script},
			}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "creating /jobs probe pod")
	defer func() {
		_ = clientset.CoreV1().Pods(agentNamespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.Eventuallyf(t, func() bool {
		p, err := clientset.CoreV1().Pods(agentNamespace).Get(ctx, pod.Name, metav1.GetOptions{})
		return err == nil && (p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed)
	}, 5*time.Minute, 5*time.Second, "/jobs probe pod %s did not complete", pod.Name)

	raw, err := clientset.CoreV1().Pods(agentNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	require.NoError(t, err, "reading /jobs probe logs")
	logs := string(raw)
	return between(logs, "===CS===", "===PN==="), between(logs, "===PN===", "===END===")
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	i += len(start)
	if j := strings.Index(s[i:], end); j >= 0 {
		return s[i : i+j]
	}
	return s[i:]
}

// TestAnnotationRoutingPartition proves the routing partition at each Target
// Allocator's /jobs: the annotated monitor's job is owned only by the
// cluster-scraper TA and the bare monitor's job only by the per-node TA (neither
// double-scraped nor dropped).
func TestAnnotationRoutingPartition(t *testing.T) {
	clientset := k8sClientset(t)
	csJobs, pnJobs := taJobsPartition(t, clientset)

	require.NotEmpty(t, strings.TrimSpace(csJobs), "cluster-scraper TA /jobs returned nothing (probe could not reach it)")
	require.NotEmpty(t, strings.TrimSpace(pnJobs), "per-node TA /jobs returned nothing (probe could not reach it)")

	assert.Containsf(t, csJobs, routedJobMarker, "cluster-scraper TA should OWN the routed monitor job %q", routedJobMarker)
	assert.NotContainsf(t, csJobs, defaultJobMarker, "cluster-scraper TA must NOT own the bare monitor %q", defaultJobMarker)
	assert.Containsf(t, pnJobs, defaultJobMarker, "per-node TA should OWN the bare monitor job %q", defaultJobMarker)
	assert.NotContainsf(t, pnJobs, routedJobMarker, "per-node TA must NOT own the routed monitor %q", routedJobMarker)
}
