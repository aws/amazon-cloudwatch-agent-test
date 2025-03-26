// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//------------------------------------------------------------------------------
// Constants
//------------------------------------------------------------------------------

const (
	Wait                    = 10 * time.Minute
	WaitForResourceCreation = 2 * time.Minute
	interval                = 30 * time.Second
)

//------------------------------------------------------------------------------
// Resource Functions
//------------------------------------------------------------------------------

func VerifyPodEnvironment(t *testing.T, clientset *kubernetes.Clientset, deploymentName string, requiredEnvVars map[string]string) {
	pods, err := clientset.CoreV1().Pods("test").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deploymentName),
		FieldSelector: "status.phase=Running",
	})
	require.NoError(t, err, "Error getting pods for deployment")
	require.NotEmpty(t, pods.Items, "No pods found for deployment")

	remainingEnvVars := make(map[string]string)
	for k, v := range requiredEnvVars {
		remainingEnvVars[k] = v
	}

	for _, container := range pods.Items[0].Spec.Containers {
		for _, envVar := range container.Env {
			if expectedValue, exists := remainingEnvVars[envVar.Name]; exists {
				require.Equal(t, expectedValue, envVar.Value,
					fmt.Sprintf("Unexpected value for environment variable %s in container %s",
						envVar.Name, container.Name))
				delete(remainingEnvVars, envVar.Name)
			}
		}
	}

	require.Empty(t, remainingEnvVars, "Not all required environment variables were found in the pod")
}

func VerifyAgentResources(t *testing.T, clientset *kubernetes.Clientset, configKeyword string) {
	daemonSet, err := clientset.AppsV1().DaemonSets("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent DaemonSet")
	require.NotNil(t, daemonSet, "CloudWatch Agent DaemonSet not found")

	configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent ConfigMap")
	require.NotNil(t, configMap, "CloudWatch Agent ConfigMap not found")

	cwConfig, exists := configMap.Data["cwagentconfig.json"]
	require.True(t, exists, "cwagentconfig.json not found in ConfigMap")
	require.Contains(t, cwConfig, configKeyword, fmt.Sprintf("%s configuration not found in cwagentconfig.json", configKeyword))

	service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent Service")
	require.NotNil(t, service, "CloudWatch Agent Service not found")

	serviceAccount, err := clientset.CoreV1().ServiceAccounts("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent Service Account")
	require.NotNil(t, serviceAccount, "CloudWatch Agent Service Account not found")
}
func GetPodList(t *testing.T, clientset *kubernetes.Clientset, namespace string, name string) v1.PodList {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", name),
	})
	require.NoError(t, err, "Error getting Pods")
	return *pods

}

//------------------------------------------------------------------------------
// Metric Functions
//------------------------------------------------------------------------------

func ValidateMetrics(t *testing.T, metrics []string, namespace string) {
	for _, metric := range metrics {
		t.Run(metric, func(t *testing.T) {
			awsservice.ValidateMetricWithTest(t, metric, namespace, nil, 5, interval)
		})
	}
}

func VerifyMetricAboveZero(t *testing.T, metricName string, namespace string) {
	startTime := time.Now().Add(-Wait)
	endTime := time.Now()

	aboveZero, err := awsservice.CheckMetricAboveZero(
		metricName,
		namespace,
		startTime,
		endTime,
		60,
	)
	require.NoError(t, err, "Failed to check metric above zero")
	require.True(t, aboveZero, fmt.Sprintf("Expected non-zero %s after applying traffic", metricName))
}
