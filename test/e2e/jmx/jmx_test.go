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
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//------------------------------------------------------------------------------
// Constants and Variables
//------------------------------------------------------------------------------

const (
	wait     = 5 * time.Minute
	interval = 30 * time.Second
)

var (
	nodeNames []string
	env       *environment.MetaData
)

//------------------------------------------------------------------------------
// Test Registry Maps
//------------------------------------------------------------------------------

var testMetricsRegistry = map[string][]func(*testing.T){
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

var testResourcesRegistry = []func(*testing.T, *kubernetes.Clientset){
	testAgentResources,
	testJMXResources,
}

//------------------------------------------------------------------------------
// Test Setup
//------------------------------------------------------------------------------

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestMain(m *testing.M) {
	flag.Parse()

	// Added this to prevent running tests when we pass in "NO_MATCH"
	if flag.Lookup("test.run").Value.String() == "NO_MATCH" {
		os.Exit(0)
	}
	env = environment.GetEnvironmentMetaData()

	// Destroy K8s resources if terraform destroy
	if env.Destroy {
		if err := common.DestroyResources(env); err != nil {
			fmt.Printf("Failed to delete kubernetes resources: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Configure AWS clients and create K8s resources
	if err := common.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
	}

	// Get names of nodes so they can be used as dimensions to check for metrics
	eksInstances, err := awsservice.GetEKSInstances(env.EKSClusterName)
	if err != nil || len(eksInstances) == 0 {
		fmt.Printf("Failed to get EKS instances: %v", err)
		os.Exit(1)
	}

	for _, instance := range eksInstances {
		if instance.InstanceName != nil {
			nodeNames = append(nodeNames, *instance.InstanceName)
		}
	}

	os.Exit(m.Run())
}

//------------------------------------------------------------------------------
// Main Test Functions
//------------------------------------------------------------------------------

func TestAll(t *testing.T) {
	t.Run("Resources", func(t *testing.T) {
		testResources(t)
	})

	// Don't run metric tests if resource tests fail
	if !t.Failed() {
		t.Run("Metrics", func(t *testing.T) {
			testMetrics(t)
		})
	}
}

func testResources(t *testing.T) {
	tests := testResourcesRegistry

	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err, "Error building kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "Error creating clientset")

	for _, testFunc := range tests {
		testFunc(t, clientset)
	}
}

func testMetrics(t *testing.T) {
	configFile := filepath.Base(env.AgentConfig)
	tests := testMetricsRegistry[configFile]

	fmt.Println("waiting for metrics to propagate...")
	time.Sleep(wait)

	for _, testFunc := range tests {
		testFunc(t)
	}
}

//------------------------------------------------------------------------------
// Resource Test Functions
//------------------------------------------------------------------------------

func testAgentResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_agent_resources", func(t *testing.T) {
		daemonSet, err := clientset.AppsV1().DaemonSets("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
		require.NoError(t, err, "Error getting CloudWatch Agent DaemonSet")
		require.NotNil(t, daemonSet, "CloudWatch Agent DaemonSet not found")

		configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
		require.NoError(t, err, "Error getting CloudWatch Agent ConfigMap")
		require.NotNil(t, configMap, "CloudWatch Agent ConfigMap not found")

		cwConfig, exists := configMap.Data["cwagentconfig.json"]
		require.True(t, exists, "cwagentconfig.json not found in ConfigMap")
		require.Contains(t, cwConfig, `"jmx"`, "JMX configuration not found in cwagentconfig.json")

		service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
		require.NoError(t, err, "Error getting CloudWatch Agent Service")
		require.NotNil(t, service, "CloudWatch Agent Service not found")

		serviceAccount, err := clientset.CoreV1().ServiceAccounts("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
		require.NoError(t, err, "Error getting CloudWatch Agent Service Account")
		require.NotNil(t, serviceAccount, "CloudWatch Agent Service Account not found")
	})
}

func testJMXResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_jmx_resources", func(t *testing.T) {
		deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yaml")
		pods, err := clientset.CoreV1().Pods("test").List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", deploymentName),
			FieldSelector: "status.phase=Running",
		})
		require.NoError(t, err, "Error getting pods for deployment")
		require.NotEmpty(t, pods.Items, "No pods found for deployment")

		var jmxTargetSystem string
		switch filepath.Base(env.AgentConfig) {
		case "jvm_tomcat.json", "containerinsights.json":
			jmxTargetSystem = "jvm,tomcat"
		case "kafka.json":
			jmxTargetSystem = "kafka"
		}

		requiredEnvVars := map[string]string{
			"OTEL_EXPORTER_OTLP_PROTOCOL":            "http/protobuf",
			"OTEL_METRICS_EXPORTER":                  "none",
			"OTEL_LOGS_EXPORTER":                     "none",
			"OTEL_TRACES_EXPORTER":                   "none",
			"OTEL_AWS_JMX_EXPORTER_METRICS_ENDPOINT": "http://cloudwatch-agent.amazon-cloudwatch:4314/v1/metrics",
			"OTEL_JMX_TARGET_SYSTEM":                 jmxTargetSystem,
			"JAVA_TOOL_OPTIONS":                      "-javaagent:/otel-auto-instrumentation-java/javaagent.jar",
		}

		for _, container := range pods.Items[0].Spec.Containers {
			for _, envVar := range container.Env {
				if expectedValue, exists := requiredEnvVars[envVar.Name]; exists {
					require.Equal(t, expectedValue, envVar.Value, fmt.Sprintf("Unexpected value for environment variable %s in container %s", envVar.Name, container.Name))
					delete(requiredEnvVars, envVar.Name)
				}
			}
		}

		require.Empty(t, requiredEnvVars, "Not all required environment variables were found in the pod")
	})
}

//------------------------------------------------------------------------------
// Metric Test Functions
//------------------------------------------------------------------------------

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		validateMetrics(t, []string{
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
			"tomcat.traffic",
			"tomcat.sessions",
			"tomcat.errors",
			"tomcat.request_count",
			"tomcat.max_time",
			"tomcat.processing_time",
			"tomcat.threads",
		}, "JVM_TOMCAT_E2E")
	})
}

func testTomcatSessions(t *testing.T) {
	t.Run("verify_tomcat_sessions", func(t *testing.T) {
		generateTraffic(t)
		time.Sleep(wait)
		verifyMetricAboveZero(t, "tomcat.sessions", "JVM_TOMCAT_E2E")
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		validateMetrics(t, []string{
			"kafka.message.count",
			"kafka.request.count",
			"kafka.request.failed",
			"kafka.request.time.total",
			"kafka.request.time.50p",
			"kafka.request.time.99p",
			"kafka.request.time.avg",
			"kafka.consumer.fetch-rate",
			"kafka.consumer.total.bytes-consumed-rate",
			"kafka.consumer.total.records-consumed-rate",
			"kafka.producer.io-wait-time-ns-avg",
			"kafka.producer.outgoing-byte-rate",
			"kafka.producer.response-rate",
		}, "KAFKA_E2E")
	})
}

func testContainerInsightsMetrics(t *testing.T) {
	t.Run("verify_containerinsights_metrics", func(t *testing.T) {
		validateMetrics(t, []string{
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
		}, "ContainerInsights/Prometheus")
	})
}

func testTomcatRejectedSessions(t *testing.T) {
	t.Run("verify_catalina_manager_rejectedsessions", func(t *testing.T) {
		generateTraffic(t)
		time.Sleep(wait)
		verifyMetricAboveZero(t, "catalina_manager_rejectedsessions", "ContainerInsights/Prometheus")
	})
}

//------------------------------------------------------------------------------
// Helper Functions
//------------------------------------------------------------------------------

func validateMetrics(t *testing.T, metrics []string, namespace string) {
	for _, metric := range metrics {
		t.Run(metric, func(t *testing.T) {
			awsservice.ValidateMetricWithTest(t, metric, namespace, nil, 5, interval)
		})
	}
}

func generateTraffic(t *testing.T) {
	cmd := exec.Command("kubectl", "get", "svc", "tomcat-service", "-n", "test", "-o", "jsonpath='{.status.loadBalancer.ingress[0].hostname}'")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Error getting LoadBalancer URL")

	lbURL := strings.Trim(string(output), "'")
	require.NotEmpty(t, lbURL, "LoadBalancer URL failed to format")

	for i := 0; i < 5; i++ {
		resp, err := http.Get(fmt.Sprintf("http://%s/webapp/index.jsp", lbURL))
		if err != nil {
			t.Logf("Request attempt %d failed: %v", i+1, err)
			continue
		}
		require.NoError(t, resp.Body.Close(), "Failed to close response body")
	}
}

func verifyMetricAboveZero(t *testing.T, metricName, namespace string) {
	startTime := time.Now().Add(-wait)
	endTime := time.Now()

	aboveZero, err := awsservice.CheckMetricAboveZero(
		metricName,
		namespace,
		startTime,
		endTime,
		60,
		nodeNames,
	)
	require.NoError(t, err, "Failed to check metric above zero")
	require.True(t, aboveZero, fmt.Sprintf("Expected non-zero %s after applying traffic", metricName))
}
