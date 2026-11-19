// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package load

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	fbNamespace = "amazon-cloudwatch"
	fbSAUser    = "system:serviceaccount:amazon-cloudwatch:fluent-bit"
)

func getClients(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	require.NotEmpty(t, kubeconfig, "KUBECONFIG not set")
	rc, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err, "building kubeconfig")
	cs, err := kubernetes.NewForConfig(rc)
	require.NoError(t, err, "creating clientset")
	return cs
}

func setFluentBitKubeletMode(t *testing.T, cs *kubernetes.Clientset, mode string) {
	t.Helper()
	ctx := context.Background()
	const key = "application-log.conf"
	cm, err := cs.CoreV1().ConfigMaps(fbNamespace).Get(ctx, "fluent-bit-config", metav1.GetOptions{})
	require.NoError(t, err, "get fluent-bit-config")
	conf, ok := cm.Data[key]
	require.Truef(t, ok, "configmap missing %s", key)

	lines := strings.Split(conf, "\n")
	for i, ln := range lines {
		if strings.Contains(ln, "Use_Kubelet") {
			lines[i] = "            Use_Kubelet         " + mode
		}
	}
	cm.Data[key] = strings.Join(lines, "\n")
	_, err = cs.CoreV1().ConfigMaps(fbNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	require.NoError(t, err, "update fluent-bit-config")

	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"test/restartedAt":"%s"}}}}}`,
		time.Now().Format(time.RFC3339Nano))
	_, err = cs.AppsV1().DaemonSets(fbNamespace).Patch(ctx, "fluent-bit",
		k8stypes.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	require.NoError(t, err, "restart fluent-bit daemonset")
	waitDaemonSetReady(t, cs, "fluent-bit", 5*time.Minute)
}

func waitDaemonSetReady(t *testing.T, cs *kubernetes.Clientset, name string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ds, err := cs.AppsV1().DaemonSets(fbNamespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil && ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberReady == ds.Status.DesiredNumberScheduled &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled {
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Logf("daemonset %s not fully ready within %s (continuing)", name, timeout)
}

func waitForGenerator(t *testing.T, cs *kubernetes.Clientset, want int) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		d, err := cs.AppsV1().Deployments("default").Get(ctx, "metadata-load-generator", metav1.GetOptions{})
		if err == nil && int(d.Status.ReadyReplicas) >= want*8/10 {
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Logf("generator not fully ready within timeout (continuing)")
}

// apiserverRequestsByFluentBit counts, from the EKS control-plane audit log, the
// completed apiserver pod/node requests attributable to the fluent-bit service
// account during [start,end]. This isolates fluent-bit's own API-server load and
// aggregates across apiserver instances (unlike an endpoint /metrics scrape).
func apiserverRequestsByFluentBit(t *testing.T, cluster string, start, end time.Time) int {
	t.Helper()
	group := fmt.Sprintf("/aws/eks/%s/cluster", cluster)
	// allow audit events to propagate to CloudWatch Logs
	time.Sleep(90 * time.Second)
	query := fmt.Sprintf(`filter stage = "ResponseComplete"
| filter user.username = "%s"
| filter objectRef.resource = "pods" or objectRef.resource = "nodes"
| stats count() as reqs`, fbSAUser)

	results, err := awsservice.GetLogQueryResults(group, start.Unix(), end.Unix(), query)
	if err != nil {
		t.Logf("audit log query failed for %s: %v", group, err)
		return -1
	}
	if len(results) == 0 {
		return 0
	}
	for _, f := range results[0] {
		if f.Field != nil && *f.Field == "reqs" && f.Value != nil {
			n, _ := strconv.Atoi(*f.Value)
			return n
		}
	}
	return 0
}

func fractionEnriched(t *testing.T, cluster string) float64 {
	t.Helper()
	group := fmt.Sprintf("/aws/containerinsights/%s/application", cluster)
	if !awsservice.IsLogGroupExistsWithRetry(group, 30, 10*time.Second) {
		t.Logf("log group %s not found", group)
		return 0
	}
	streams := awsservice.GetLogStreams(group)
	if len(streams) == 0 {
		t.Logf("no log streams in %s", group)
		return 0
	}
	since := time.Now().Add(-10 * time.Minute)
	until := time.Now()
	var total, enriched int
	for _, s := range streams {
		events, err := awsservice.GetLogsSince(group, *s.LogStreamName, &since, &until)
		if err != nil {
			continue
		}
		for _, e := range events {
			total++
			if strings.Contains(*e.Message, `"kubernetes"`) ||
				(strings.Contains(*e.Message, `"pod_name"`) && strings.Contains(*e.Message, `"namespace_name"`)) {
				enriched++
			}
		}
	}
	if total == 0 {
		t.Logf("no log events sampled from %s", group)
		return 0
	}
	t.Logf("enrichment sample: enriched=%d total=%d", enriched, total)
	return float64(enriched) / float64(total)
}
