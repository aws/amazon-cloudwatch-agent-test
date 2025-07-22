// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
)

//------------------------------------------------------------------------------
// Variables
//------------------------------------------------------------------------------

var (
	env *environment.MetaData
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
		if err := e2e.DestroyResources(env); err != nil {
			fmt.Printf("Failed to delete kubernetes resources: %v\n", err)
		}
		os.Exit(0)
	}

	// Configure AWS clients and create K8s resources
	if err := e2e.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
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
	time.Sleep(e2e.Wait)

	for _, testFunc := range tests {
		testFunc(t)
	}
}

//------------------------------------------------------------------------------
// Resource Test Functions
//------------------------------------------------------------------------------

func testAgentResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_agent_resources", func(t *testing.T) {
		time.Sleep(e2e.WaitForResourceCreation)
		e2e.VerifyAgentResourcesDaemonSet(t, clientset, "jmx")
	})
}

func testJMXResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_jmx_resources", func(t *testing.T) {
		deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yaml")
		time.Sleep(e2e.WaitForResourceCreation)

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
			"JAVA_TOOL_OPTIONS":                      " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
		}

		e2e.VerifyPodEnvironment(t, clientset, deploymentName, requiredEnvVars)
	})
}

//------------------------------------------------------------------------------
// Metric Test Functions
//------------------------------------------------------------------------------

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		e2e.ValidateMetrics(t, []string{
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
		time.Sleep(e2e.Wait)
		e2e.VerifyMetricAboveZero(t, "tomcat.sessions", "JVM_TOMCAT_E2E")
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		e2e.ValidateMetrics(t, []string{
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
		e2e.ValidateMetrics(t, []string{
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
		time.Sleep(e2e.Wait)
		e2e.VerifyMetricAboveZero(t, "catalina_manager_rejectedsessions", "ContainerInsights/Prometheus")
	})
}
