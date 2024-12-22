// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var testRegistry = map[string][]func(*testing.T){
	"prometheus.json": {
		testPrometheusMetrics,
	},
}

func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.run").Value.String() == "NO_MATCH" {
		os.Exit(0)
	}
	env := environment.GetEnvironmentMetaData()

	if env.Region != "us-west-2" {
		if err := awsservice.ConfigureAWSClients(env.Region); err != nil {
			fmt.Printf("Failed to reconfigure AWS clients: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("AWS clients reconfigured to use region: %s\n", env.Region)
	} else {
		fmt.Printf("Using default testing region: us-west-2\n")
	}

	fmt.Println("Starting Helm installation...")
	if err := ApplyHelm(env); err != nil {
		fmt.Printf("Failed to apply Helm: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for metrics to propagate...")
	time.Sleep(5 * time.Minute)

	os.Exit(m.Run())
}

func ApplyHelm(env *environment.MetaData) error {
	updateKubeconfig := exec.Command("aws", "eks", "update-kubeconfig", "--name", env.EKSClusterName)
	output, err := updateKubeconfig.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}

	helm := []string{
		"helm", "upgrade", "--install", "amazon-cloudwatch-observability",
		filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability"),
		"--values", filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability", "values.yaml"),
		"--set", fmt.Sprintf("clusterName=%s", env.EKSClusterName),
		"--set", fmt.Sprintf("region=%s", env.Region),
		"--set", fmt.Sprintf("agent.image.repository=%s", env.CloudwatchAgentRepository),
		"--set", fmt.Sprintf("agent.image.tag=%s", env.CloudwatchAgentTag),
		"--set", fmt.Sprintf("agent.image.repositoryDomainMap.public=%s", env.CloudwatchAgentRepositoryURL),
		"--set", fmt.Sprintf("manager.image.repository=%s", env.CloudwatchAgentOperatorRepository),
		"--set", fmt.Sprintf("manager.image.tag=%s", env.CloudwatchAgentOperatorTag),
		"--set", fmt.Sprintf("manager.image.repositoryDomainMap.public=%s", env.CloudwatchAgentOperatorRepositoryURL),
		"--set", fmt.Sprintf("agent.prometheus.targetAllocator.image.repository=%s", env.CloudwatchAgentTargetAllocatorRepository),
		"--set", fmt.Sprintf("agent.prometheus.targetAllocator.image.tag=%s", env.CloudwatchAgentTargetAllocatorTag),
		"--set", fmt.Sprintf("agent.prometheus.targetAllocator.image.repositoryDomainMap.public=%s", env.CloudwatchAgentTargetAllocatorRepositoryURL),
		"--set", "agent.prometheus.targetAllocator.enabled=true",
		"--set", "agent.mode=statefulset",
		"--namespace", "amazon-cloudwatch",
		"--create-namespace",
	}

	if env.AgentConfig != "" {
		agentConfigContent, err := os.ReadFile(env.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
		helm = append(helm, "--set-json", fmt.Sprintf("agent.config=%s", string(agentConfigContent)))
	}

	if env.PrometheusConfig != "" {
		prometheusConfigContent, err := os.ReadFile(env.PrometheusConfig)
		if err != nil {
			return fmt.Errorf("failed to read prometheus config file: %w", err)
		}

		var configData interface{}
		if err := yaml.Unmarshal(prometheusConfigContent, &configData); err != nil {
			return fmt.Errorf("failed to parse prometheus config: %w", err)
		}

		jsonConfig, err := json.Marshal(configData)
		if err != nil {
			return fmt.Errorf("failed to convert config to JSON: %w", err)
		}

		helm = append(helm, "--set-json", fmt.Sprintf("agent.prometheus.config=%s", string(jsonConfig)))
	}

	helmUpgrade := exec.Command(helm[0], helm[1:]...)
	helmUpgrade.Stdout = os.Stdout
	helmUpgrade.Stderr = os.Stderr
	if err := helmUpgrade.Run(); err != nil {
		return fmt.Errorf("failed to install Helm release: %w", err)
	}

	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	time.Sleep(300 * time.Second)

	deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yaml")

	apply := exec.Command("kubectl", "apply", "-f", env.SampleApp)
	output, err = apply.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply sample app: %w\nOutput: %s", err, output)
	}

	wait := exec.Command("kubectl", "wait", "--for=condition=available", "--timeout=300s", fmt.Sprintf("deployment/%s", deploymentName), "-n", "default")
	output, err = wait.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for deployment %s: %w\nOutput: %s", deploymentName, err, output)
	}

	return nil
}

func TestResources(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	if err != nil {
		t.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// First verify resources exist
	deployment, err := clientset.AppsV1().Deployments("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting Target Allocator Deployment: %v", err)
	}
	if deployment == nil {
		t.Error("Target Allocator Deployment not found")
	}

	service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator-service", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting Target Allocator Service: %v", err)
	}
	if service == nil {
		t.Error("Target Allocator Service not found")
	}

	promConfigMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-prometheus-config", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting Prometheus ConfigMap: %v", err)
	}
	if promConfigMap == nil {
		t.Error("Prometheus ConfigMap not found")
	}

	targetAllocatorConfigMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting Target Allocator ConfigMap: %v", err)
	}
	if targetAllocatorConfigMap == nil {
		t.Error("Target Allocator ConfigMap not found")
	}

	// Update Helm release to disable target allocator
	helm := []string{
		"helm", "upgrade", "amazon-cloudwatch-observability",
		filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability"),
		"--set", fmt.Sprintf("region=%s", env.Region),
		"--set", "agents.prometheus.targetAllocator.enabled=false",
		"--namespace", "amazon-cloudwatch",
	}

	helmCmd := exec.Command(helm[0], helm[1:]...)
	if output, err := helmCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to update Helm release: %v\nOutput: %s", err, output)
	}

	time.Sleep(30 * time.Second)

	// Now verify resources don't exist
	deployment, err = clientset.AppsV1().Deployments("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator", metav1.GetOptions{})
	if err == nil {
		t.Error("Target Allocator Deployment still exists when it should be deleted")
	}

	service, err = clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator-service", metav1.GetOptions{})
	if err == nil {
		t.Error("Target Allocator Service still exists when it should be deleted")
	}

	promConfigMap, err = clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-prometheus-config", metav1.GetOptions{})
	if err == nil {
		t.Error("Prometheus ConfigMap still exists when it should be deleted")
	}

	targetAllocatorConfigMap, err = clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent-w-prom-target-allocator", metav1.GetOptions{})
	if err == nil {
		t.Error("Target Allocator ConfigMap still exists when it should be deleted")
	}
}

func TestMetrics(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	configFile := filepath.Base(env.AgentConfig)

	tests, exists := testRegistry[configFile]
	if !exists {
		t.Skipf("No tests registered for config file: %s", configFile)
		return
	}

	for _, testFunc := range tests {
		testFunc(t)
	}
}

func testPrometheusMetrics(t *testing.T) {
	t.Run("verify_prometheus_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"memcached_commands_total", "PROMETHEUS_E2E"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
			})
		}
	})
}
