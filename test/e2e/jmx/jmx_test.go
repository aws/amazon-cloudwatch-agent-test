package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestMain(m *testing.M) {
	flag.Parse()
	env := environment.GetEnvironmentMetaData()

	if env.Region != "us-west-2" {
		if err := awsservice.ReconfigureAWSClients(env.Region); err != nil {
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
	updateKubeconfigCmd := exec.Command("aws", "eks", "update-kubeconfig", "--name", env.EKSClusterName)
	output, err := updateKubeconfigCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}

	helmCmd := []string{
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
		"--namespace", "amazon-cloudwatch",
		"--create-namespace",
	}

	if env.AgentConfig != "" {
		agentConfigContent, err := os.ReadFile(env.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
		helmCmd = append(helmCmd, "--set-json", fmt.Sprintf("agent.config=%s", string(agentConfigContent)))
	}

	helmUpgradeCmd := exec.Command(helmCmd[0], helmCmd[1:]...)
	helmUpgradeCmd.Stdout = os.Stdout
	helmUpgradeCmd.Stderr = os.Stderr
	if err := helmUpgradeCmd.Run(); err != nil {
		return fmt.Errorf("failed to install Helm release: %w", err)
	}

	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	time.Sleep(300 * time.Second)

	applyCmd := exec.Command("kubectl", "apply", "-f", env.SampleApp)
	output, err = applyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply sample app: %w\nOutput: %s", err, output)
	}

	waitCmd := exec.Command("kubectl", "wait", "--for=condition=available", "--timeout=300s", "deployment", "--all", "-n", "default")
	output, err = waitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for deployments: %w\nOutput: %s", err, output)
	}

	return nil
}

func TestResources(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	if err != nil {
		t.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Error creating Kubernetes client: %v", err)
	}

	daemonSet, err := clientset.AppsV1().DaemonSets("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent DaemonSet: %v", err)
	}
	if daemonSet == nil {
		t.Error("CloudWatch Agent DaemonSet not found")
	}

	configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent ConfigMap: %v", err)
	}
	if configMap == nil {
		t.Error("CloudWatch Agent ConfigMap not found")
	}
}

func TestMetrics(t *testing.T) {
	metricsToCheck := []struct {
		name      string
		namespace string
	}{
		{"tomcat.traffic", "JVM_TOMCAT_E2E"},
	}

	for _, metric := range metricsToCheck {
		t.Run(metric.name, func(t *testing.T) {
			awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
		})
	}
}
