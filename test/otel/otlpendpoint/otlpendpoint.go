// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package otlpendpoint holds the helpers shared by the suites that test the Helm chart's OTLP endpoint:
// test/otel/otlp, test/otel/otlp_disabled and the OTLP cases of test/otel/filters on EKS, and the OTLP
// test of test/azure/aks/containerinsights on AKS. The HTTP senders they read run
// terraform/otlp_service_sender.sh.
package otlpendpoint

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// AgentNamespace and AgentName are the chart's namespace and node agent. The node agent's Service,
	// which the operator creates from the agent's receivers, has the agent's name.
	AgentNamespace = "amazon-cloudwatch"
	AgentName      = "cloudwatch-agent"

	// GRPCPort and HTTPPort are the chart's default OTLP ports.
	GRPCPort = 4317
	HTTPPort = 4318

	// Solution is the cloudwatch.solution scope attribute the chart's OTLP pipelines set.
	Solution = "otel-otlp"

	// MetricName is the counter otlp_service_sender.sh sends by default, and CaseAttribute the attribute
	// it sets on every data point, log record and span.
	MetricName    = "otlp_e2e_requests"
	CaseAttribute = "otlp_e2e_case"

	sendPrefix = "otlp-e2e-send "
)

// Signals are the OTLP signals otlp_service_sender.sh posts, in the names of their /v1 paths.
var Signals = []string{"metrics", "logs", "traces"}

// LogBody is the body of every log record otlp_service_sender.sh sends for case.
func LogBody(senderCase string) string {
	return "otlp-e2e " + senderCase
}

// LogGroup is the log group the chart's OTLP logs pipeline writes to when the sender sets none.
func LogGroup(cluster string) string {
	return fmt.Sprintf("/aws/cwagent/%s/otlp", cluster)
}

// KubeClient returns a client for the cluster in the kubeconfig that Terraform writes for kubectl.
func KubeClient() (*kubernetes.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}

// ServicePorts returns the port numbers of the node agent's Service. found is false when there is no
// Service: the operator creates it only when the agent's configuration yields a port to open, so with
// the OTLP endpoint off there may be none.
func ServicePorts(ctx context.Context, cs kubernetes.Interface) (ports []int32, found bool, err error) {
	svc, err := cs.CoreV1().Services(AgentNamespace).Get(ctx, AgentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	ports = make([]int32, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, p.Port)
	}
	return ports, true, nil
}

// AgentHostPorts returns the ports the node agent DaemonSet's containers open on the node. The agent
// runs on the host network, where a container port is a host port; otherwise only declared host ports
// count.
func AgentHostPorts(ctx context.Context, cs kubernetes.Interface) ([]int32, error) {
	ds, err := cs.AppsV1().DaemonSets(AgentNamespace).Get(ctx, AgentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return hostPorts(ds.Spec.Template.Spec), nil
}

func hostPorts(spec corev1.PodSpec) []int32 {
	var ports []int32
	for _, c := range spec.Containers {
		for _, p := range c.Ports {
			switch {
			case spec.HostNetwork:
				ports = append(ports, p.ContainerPort)
			case p.HostPort != 0:
				ports = append(ports, p.HostPort)
			}
		}
	}
	return ports
}

// RunningPods returns the running pods in namespace that match the label selector.
func RunningPods(ctx context.Context, cs kubernetes.Interface, namespace, selector string) ([]corev1.Pod, error) {
	list, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var pods []corev1.Pod
	for _, pod := range list.Items {
		if pod.Status.Phase == corev1.PodRunning {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

// SendStatuses reads the log of a sender pod and returns, per signal, the HTTP status of every post
// ("000" when the connection failed).
func SendStatuses(ctx context.Context, cs kubernetes.Interface, namespace, pod string) (map[string][]string, error) {
	raw, err := cs.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	return ParseSendStatuses(string(raw)), nil
}

// ParseSendStatuses parses the "otlp-e2e-send <signal> <status>" lines of a sender log.
func ParseSendStatuses(log string) map[string][]string {
	statuses := map[string][]string{}
	scanner := bufio.NewScanner(strings.NewReader(log))
	for scanner.Scan() {
		line, ok := strings.CutPrefix(scanner.Text(), sendPrefix)
		if !ok {
			continue
		}
		if fields := strings.Fields(line); len(fields) == 2 {
			statuses[fields[0]] = append(statuses[fields[0]], fields[1])
		}
	}
	return statuses
}

// NodeInstanceID returns the EC2 instance ID from a node's provider ID (aws:///<zone>/<instance id>).
func NodeInstanceID(node corev1.Node) string {
	parts := strings.Split(node.Spec.ProviderID, "/")
	return parts[len(parts)-1]
}
