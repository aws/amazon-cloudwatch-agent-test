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

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

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

var testResourcesRegistry = []func(*testing.T){
	testJMXResources,
}

var nodeNames []string

func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.run").Value.String() == "NO_MATCH" {
		os.Exit(0)
	}
	env := environment.GetEnvironmentMetaData()

	if env.Destroy {
		if err := common.DestroyResources(env); err != nil {
			fmt.Printf("Failed to delete kubernetes resources: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := common.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
	}

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

func TestAll(t *testing.T) {
	t.Run("Resources", func(t *testing.T) {
		testResources(t)
	})

	if !t.Failed() {
		t.Run("Metrics", func(t *testing.T) {
			testMetrics(t)
		})
	}
}

func testResources(t *testing.T) {
	tests := testResourcesRegistry

	for _, testFunc := range tests {
		testFunc(t)
	}
}

func testJMXResources(t *testing.T) {
	t.Run("verify_jmx_resources", func(t *testing.T) {
		config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
		require.NoError(t, err, "Error building kubeconfig")

		clientset, err := kubernetes.NewForConfig(config)
		require.NoError(t, err, "Error creating clientset")

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

func testMetrics(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	configFile := filepath.Base(env.AgentConfig)

	tests, exists := testMetricsRegistry[configFile]
	if !exists {
		t.Skipf("No tests registered for config file: %s", configFile)
		return
	}

	fmt.Println("Waiting for metrics to propagate...")
	time.Sleep(10 * time.Minute)

	for _, testFunc := range tests {
		testFunc(t)
	}
}

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		metricsToCheck := []string{
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
		}

		for _, metric := range metricsToCheck {
			t.Run(metric, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric, "JVM_TOMCAT_E2E", nil, 5, 30*time.Second)
			})
		}
	})
}

func testTomcatSessions(t *testing.T) {
	t.Run("verify_tomcat_sessions", func(t *testing.T) {
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

		time.Sleep(5 * time.Minute)

		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()

		aboveZero, err := awsservice.CheckMetricAboveZero(
			"tomcat.sessions",
			"JVM_TOMCAT_E2E",
			startTime,
			endTime,
			60,
			nodeNames,
		)
		require.NoError(t, err, "Failed to check metric above zero")
		require.True(t, aboveZero, "Expected non-zero tomcat.sessions after applying traffic")
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		metricsToCheck := []string{
			"kafka.message.count",
			"kafka.request.count",
			"kafka.request.failed",
			"kafka.request.time.total",
			"kafka.request.time.50p",
			"kafka.request.time.99p",
			"kafka.request.time.avg",
			"kafka.producer.io-wait-time-ns-avg",
			"kafka.producer.outgoing-byte-rate",
			"kafka.producer.request-rate",
			"kafka.producer.response-rate",
			"kafka.consumer.fetch-rate",
			"kafka.consumer.total.bytes-consumed-rate",
			"kafka.consumer.total.records-consumed-rate",
		}

		for _, metric := range metricsToCheck {
			t.Run(metric, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric, "KAFKA_E2E", nil, 5, 30*time.Second)
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
				awsservice.ValidateMetricWithTest(t, metric, "ContainerInsights/Prometheus", nil, 5, 30*time.Second)
			})
		}
	})
}

func testTomcatRejectedSessions(t *testing.T) {
	t.Run("verify_catalina_manager_rejectedsessions", func(t *testing.T) {
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

		time.Sleep(5 * time.Minute)

		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()

		aboveZero, err := awsservice.CheckMetricAboveZero(
			"catalina_manager_rejectedsessions",
			"ContainerInsights/Prometheus",
			startTime,
			endTime,
			60,
			nodeNames,
		)
		require.NoError(t, err, "Failed to check metric above zero")
		require.True(t, aboveZero, "Expected non-zero catalina_manager_rejectedsessions after applying traffic")
	})
}
