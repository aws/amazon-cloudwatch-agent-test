//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// taClientCertSecret holds the client certificate the agents present to the
// Target Allocator. The TA's HTTPS server requires and verifies a client
// certificate (tls.RequireAndVerifyClientCert), so the probe must present it too.
const taClientCertSecret = "amazon-cloudwatch-observability-agent-ta-client-cert"

// taProbeCertDir is where the probe pod mounts taClientCertSecret.
const taProbeCertDir = "/etc/ta-client-cert"

// taCurl is the curl invocation a probe script uses to call a Target Allocator
// over mTLS. The server certificate is not verified (-k), matching the manual
// runbook; only the client side of the handshake matters here.
const taCurl = "curl -sk --max-time 10 --cert " + taProbeCertDir + "/tls.crt --key " + taProbeCertDir + "/tls.key"

// runTAProbe runs script in a short-lived in-cluster pod that has the agents' TA
// client certificate mounted, waits for it to finish, and returns its logs. The
// Target Allocator services are only reachable inside the cluster, so the
// assertions read TA state through this probe.
func runTAProbe(t *testing.T, clientset *kubernetes.Clientset, script string) string {
	t.Helper()
	pod, err := clientset.CoreV1().Pods(agentNamespace).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "ta-jobs-probe-", Namespace: agentNamespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:         "probe",
				Image:        "curlimages/curl:8.10.1",
				Command:      []string{"sh", "-c", script},
				VolumeMounts: []corev1.VolumeMount{{Name: "ta-client-cert", MountPath: taProbeCertDir, ReadOnly: true}},
			}},
			Volumes: []corev1.Volume{{
				Name:         "ta-client-cert",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: taClientCertSecret}},
			}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "creating Target Allocator probe pod")
	defer func() {
		_ = clientset.CoreV1().Pods(agentNamespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.Eventuallyf(t, func() bool {
		p, err := clientset.CoreV1().Pods(agentNamespace).Get(ctx, pod.Name, metav1.GetOptions{})
		return err == nil && (p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed)
	}, 5*time.Minute, 5*time.Second, "Target Allocator probe pod %s did not complete", pod.Name)

	raw, err := clientset.CoreV1().Pods(agentNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	require.NoError(t, err, "reading Target Allocator probe logs")
	return string(raw)
}

// between returns the text in s between the start and end markers.
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
