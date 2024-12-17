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

func TestMain(m *testing.M) {
	flag.Parse()
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

	service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent Service: %v", err)
	}
	if service == nil {
		t.Error("CloudWatch Agent Service not found")
	}

	serviceAccount, err := clientset.CoreV1().ServiceAccounts("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent Service Account: %v", err)
	}
	if serviceAccount == nil {
		t.Error("CloudWatch Agent Service Account not found")
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

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"tomcat.traffic", "JVM_TOMCAT_E2E"},
			{"jvm.classes.loaded", "JVM_TOMCAT_E2E"},
			{"jvm.gc.collections.count", "JVM_TOMCAT_E2E"},
			{"jvm.gc.collections.elapsed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.committed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.committed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.committed", "JVM_TOMCAT_E2E"},
			{"jvm.threads.count", "JVM_TOMCAT_E2E"},
			{"tomcat.sessions", "JVM_TOMCAT_E2E"},
			{"tomcat.errors", "JVM_TOMCAT_E2E"},
			{"tomcat.request_count", "JVM_TOMCAT_E2E"},
			{"tomcat.max_time", "JVM_TOMCAT_E2E"},
			{"tomcat.processing_time", "JVM_TOMCAT_E2E"},
			{"tomcat.threads", "JVM_TOMCAT_E2E"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
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

		maxSessions, err := awsservice.GetMetricMaximum(
			"tomcat.sessions",
			"JVM_TOMCAT_E2E",
			startTime,
			endTime,
			60,
		)
		if err != nil {
			t.Errorf("Failed to get metric maximum: %v", err)
			return
		}

		if maxSessions == 0 {
			t.Error("Expected non-zero tomcat.sessions after applying traffic")
		}
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"kafka.consumer.fetch-rate", "KAFKA_E2E"},
			{"kafka.consumer.total.bytes-consumed-rate", "KAFKA_E2E"},
			{"kafka.consumer.total.records-consumed-rate", "KAFKA_E2E"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
			})
		}
	})
}

func testContainerInsightsMetrics(t *testing.T) {
	t.Run("verify_containerinsights_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"jvm_classes_loaded", "ContainerInsights/Prometheus"},
			{"jvm_threads_current", "ContainerInsights/Prometheus"},
			{"jvm_threads_daemon", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_totalswapspacesize", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_systemcpuload", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_processcpuload", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_freeswapspacesize", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_totalphysicalmemorysize", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_freephysicalmemorysize", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_openfiledescriptorcount", "ContainerInsights/Prometheus"},
			{"java_lang_operatingsystem_availableprocessors", "ContainerInsights/Prometheus"},
			{"jvm_memory_bytes_used", "ContainerInsights/Prometheus"},
			{"jvm_memory_pool_bytes_used", "ContainerInsights/Prometheus"},
			{"catalina_manager_activesessions", "ContainerInsights/Prometheus"},
			{"catalina_manager_rejectedsessions", "ContainerInsights/Prometheus"},
			{"catalina_globalrequestprocessor_requestcount", "ContainerInsights/Prometheus"},
			{"catalina_globalrequestprocessor_errorcount", "ContainerInsights/Prometheus"},
			{"catalina_globalrequestprocessor_processingtime", "ContainerInsights/Prometheus"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
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

		maxRejectedSessions, err := awsservice.GetMetricMaximum(
			"catalina_manager_rejectedsessions",
			"ContainerInsights/Prometheus",
			startTime,
			endTime,
			60,
		)
		if err != nil {
			t.Errorf("Failed to get metric maximum: %v", err)
			return
		}

		if maxRejectedSessions == 0 {
			t.Error("Expected non-zero catalina_manager_rejectedsessions after applying traffic")
		}
	})
}
