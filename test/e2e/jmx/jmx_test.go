// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	NAMESPACE_JVM_TOMCAT        = "JVM_TOMCAT_E2E"
	NAMESPACE_KAFKA             = "KAFKA_E2E"
	NAMESPACE_CONTAINERINSIGHTS = "ContainerInsights/Prometheus"
)

var testRegistry = map[string][]func(*testing.T){
	"jvm_tomcat.json": {
		testTomcatMetrics,
		testTomcatSessions,
	},
	"kafka.json": {
		testKafkaMetrics,
	},
	"containerinsights.json": {
		testContainerInsightsMetrics,
		testTomcatRejectedSessions,
	},
}

var nodeNames []string

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
		helm = append(helm, "--set-json", fmt.Sprintf("agent.config=%s", string(agentConfigContent)))
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
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err, "Error building kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "Error building kubeconfig")

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err, "Error listing nodes")

	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	daemonSet, err := clientset.AppsV1().DaemonSets("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent DaemonSet")
	require.NotNil(t, daemonSet, "CloudWatch Agent DaemonSet not found")

	configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent ConfigMap")
	require.NotNil(t, configMap, "CloudWatch Agent ConfigMap not found")

	service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent Service")
	require.NotNil(t, service, "CloudWatch Agent Service not found")

	serviceAccount, err := clientset.CoreV1().ServiceAccounts("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	require.NoError(t, err, "Error getting CloudWatch Agent Service Account")
	require.NotNil(t, serviceAccount, "CloudWatch Agent Service Account not found")
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

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		metricsToCheck := []string{
			"tomcat.traffic",
			"jvm.classes.loaded",
			"jvm.gc.collections.count",
			"jvm.gc.collections.elapsed",
			"jvm.memory.heap.init",
			"jvm.memory.heap.max",
			"jvm.memory.heap.used",
			"jvm.memory.heap.committed",
			"jvm.memory.nonheap.init",
			"jvm.memory.nonheap.max",
			"jvm.memory.nonheap.used",
			"jvm.memory.nonheap.committed",
			"jvm.memory.pool.init",
			"jvm.memory.pool.max",
			"jvm.memory.pool.used",
			"jvm.memory.pool.committed",
			"jvm.threads.count",
			"tomcat.sessions",
			"tomcat.errors",
			"tomcat.request_count",
			"tomcat.max_time",
			"tomcat.processing_time",
			"tomcat.threads",
		}

		for _, metric := range metricsToCheck {
			t.Run(metric, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric, metric, nil, 5, 1*time.Minute)
			})
		}
	})
}

func testTomcatSessions(t *testing.T) {
	t.Run("verify_tomcat_sessions", func(t *testing.T) {
		cmd := exec.Command("kubectl", "get", "svc", "tomcat-service", "-o", "jsonpath='{.status.loadBalancer.ingress[0].hostname}'")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Error getting LoadBalancer URL: %v", err)
		}

		lbURL := strings.Trim(string(output), "'")
		if lbURL == "" {
			t.Fatal("LoadBalancer URL failed to format")
		}

		for i := 0; i < 5; i++ {
			resp, err := http.Get(fmt.Sprintf("http://%s/webapp/index.jsp", lbURL))
			if err != nil {
				t.Logf("Request attempt %d failed: %v", i+1, err)
				continue
			}
			err = resp.Body.Close()
			if err != nil {
				t.Errorf("Failed to close response body: %v", err)
				return
			}
		}

		time.Sleep(5 * time.Minute)

		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()

		aboveZero, err := awsservice.CheckMetricAboveZero(
			"tomcat.sessions",
			NAMESPACE_JVM_TOMCAT,
			startTime,
			endTime,
			60,
			nodeNames,
		)
		if err != nil {
			t.Errorf("Failed to check metric above zero: %v", err)
			return
		}

		if !aboveZero {
			t.Error("Expected non-zero tomcat.sessions after applying traffic")
		}

		deleteCmd := exec.Command("kubectl", "delete", "svc", "tomcat-service")
		if output, err := deleteCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: Failed to delete load balancer service: %v\nOutput: %s", err, output)
		} else {
			t.Log("Successfully deleted load balancer service")
		}
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		metricsToCheck := []string{
			"kafka.consumer.fetch-rate",
			"kafka.consumer.total.bytes-consumed-rate",
			"kafka.consumer.total.records-consumed-rate",
		}

		for _, metric := range metricsToCheck {
			t.Run(metric, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric, NAMESPACE_KAFKA, nil, 5, 1*time.Minute)
			})
		}
	})
}

func testContainerInsightsMetrics(t *testing.T) {
	t.Run("verify_containerinsights_metrics", func(t *testing.T) {
		metricsToCheck := []string{
			"jvm_classes_loaded",
			"jvm_threads_current",
			"jvm_threads_daemon",
			"java_lang_operatingsystem_totalswapspacesize",
			"java_lang_operatingsystem_systemcpuload",
			"java_lang_operatingsystem_processcpuload",
			"java_lang_operatingsystem_freeswapspacesize",
			"java_lang_operatingsystem_totalphysicalmemorysize",
			"java_lang_operatingsystem_freephysicalmemorysize",
			"java_lang_operatingsystem_openfiledescriptorcount",
			"java_lang_operatingsystem_availableprocessors",
			"jvm_memory_bytes_used",
			"jvm_memory_pool_bytes_used",
			"catalina_manager_activesessions",
			"catalina_manager_rejectedsessions",
			"catalina_globalrequestprocessor_requestcount",
			"catalina_globalrequestprocessor_errorcount",
			"catalina_globalrequestprocessor_processingtime",
		}

		for _, metric := range metricsToCheck {
			t.Run(metric, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric, NAMESPACE_CONTAINERINSIGHTS, nil, 5, 1*time.Minute)
			})
		}
	})
}

func testTomcatRejectedSessions(t *testing.T) {
	t.Run("verify_catalina_manager_rejectedsessions", func(t *testing.T) {
		cmd := exec.Command("kubectl", "get", "svc", "tomcat-service", "-o", "jsonpath='{.status.loadBalancer.ingress[0].hostname}'")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Error getting LoadBalancer URL: %v", err)
		}

		lbURL := strings.Trim(string(output), "'")
		if lbURL == "" {
			t.Fatal("LoadBalancer URL failed to format")
		}

		for i := 0; i < 5; i++ {
			resp, err := http.Get(fmt.Sprintf("http://%s/webapp/index.jsp", lbURL))
			if err != nil {
				t.Logf("Request attempt %d failed: %v", i+1, err)
				continue
			}
			err = resp.Body.Close()
			if err != nil {
				t.Errorf("Failed to close response body: %v", err)
				return
			}
		}

		time.Sleep(5 * time.Minute)

		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()

		aboveZero, err := awsservice.CheckMetricAboveZero(
			"catalina_manager_rejectedsessions",
			NAMESPACE_CONTAINERINSIGHTS,
			startTime,
			endTime,
			60,
			nodeNames,
		)
		if err != nil {
			t.Errorf("Failed to check metric above zero: %v", err)
			return
		}

		if !aboveZero {
			t.Error("Expected non-zero catalina_manager_rejectedsessions after applying traffic")
		}

		deleteCmd := exec.Command("kubectl", "delete", "svc", "tomcat-service")
		if output, err := deleteCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: Failed to delete load balancer service: %v\nOutput: %s", err, output)
		} else {
			t.Log("Successfully deleted load balancer service")
		}
	})
}
