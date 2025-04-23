// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	//"strings"
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
	"targetallocator.json": {},
}

var testResourcesRegistry = []func(*testing.T, *kubernetes.Clientset){
	testAgentResources,
	//testTargetAllocatorDeployment,
	//testTargetAllocatorResources,
	//testTargetAllocatorScraperServer,
	//testPrometheusConfiguration,
	//testMultipleCollectors,
	//testReconciler,
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
	//if !t.Failed() {
	//	t.Run("Metrics", func(t *testing.T) {
	//		testMetrics(t)
	//	})
	//}
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

//func testMetrics(t *testing.T) {
//	configFile := filepath.Base(env.AgentConfig)
//	tests := testMetricsRegistry[configFile]
//
//	fmt.Println("waiting for metrics to propagate...")
//	time.Sleep(e2e.Wait)
//
//	for _, testFunc := range tests {
//		testFunc(t)
//	}
//}

//------------------------------------------------------------------------------
// Resource Test Functions
//------------------------------------------------------------------------------

func testAgentResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_agent_resources", func(t *testing.T) {
		time.Sleep(e2e.WaitForResourceCreation)
		e2e.VerifyAgentResources(t, clientset, "target_allocator")
	})
}

//func TestTargetAllocator(t *testing.T) {
//	clientset := setupKubernetesClientset(t)
//
//	for _, testFunc := range testTargetAllocatorRegistry {
//		testFunc(t, clientset)
//	}
//}
//
//func testTargetAllocatorDeployment(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_target_allocator_deployment", func(t *testing.T) {
//		// Test case 1
//		enabled := isTargetAllocatorEnabled(clientset)
//		validConfig := hasValidPrometheusConfig(clientset)
//		isStatefulSet := isStatefulSetMode(clientset)
//
//		require.True(t, enabled && validConfig && isStatefulSet, "Target Allocator should only be deployed when enabled, with valid config, and in statefulset mode")
//	})
//}
//
//func testTargetAllocatorResources(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_target_allocator_resources", func(t *testing.T) {
//		// Test case 2
//		time.Sleep(time.Second * 10) // Wait for resources to be created
//		verifyConfigMap(t, clientset, "target-allocator-config")
//		verifyService(t, clientset, "target-allocator-service")
//		verifyServiceAccount(t, clientset, "target-allocator-sa")
//		verifyDeployment(t, clientset, "target-allocator")
//	})
//}
//
//func testTargetAllocatorScraperServer(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_scraper_server", func(t *testing.T) {
//		// Test cases 3, 4, 5, 6
//		scraperEndpoint := getScraperServerEndpoint(clientset)
//
//		config := getScraperConfig(scraperEndpoint)
//		require.NotEmpty(t, config, "Scraper config should not be empty")
//
//		jobs := getJobs(scraperEndpoint)
//		require.NotEmpty(t, jobs, "Jobs list should not be empty")
//
//		jobPerCollector := getJobPerCollector(scraperEndpoint, "collector-1")
//		require.NotEmpty(t, jobPerCollector, "Job per collector should not be empty")
//
//		livezStatus := getEndpointStatus(scraperEndpoint + "/livez")
//		readyzStatus := getEndpointStatus(scraperEndpoint + "/readyz")
//		require.Equal(t, 200, livezStatus, "Livez endpoint should return 200")
//		require.Equal(t, 200, readyzStatus, "Readyz endpoint should return 200")
//	})
//}
//
//func testPrometheusConfiguration(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_prometheus_configuration", func(t *testing.T) {
//		// Test cases 7, 8, 9, 11, 12, 13
//		invalidConfig := getInvalidPrometheusConfig()
//		require.False(t, arePrometheusResourcesDeployed(clientset, invalidConfig), "Invalid config should not deploy Prometheus resources")
//
//		taEnabled := isTargetAllocatorEnabled(clientset)
//		configUsesTa := doesPrometheusCombineUseTargetAllocator(clientset)
//		require.Equal(t, taEnabled, configUsesTa, "Prometheus config should use TA when TA is enabled")
//
//		crWatcherEnabled := isPrometheusCRWatcherEnabled(clientset)
//		require.True(t, crWatcherEnabled, "Prometheus CR watcher should be enabled")
//
//		require.True(t, doesPrometheusConfigContainDollarSign(clientset), "Prometheus config should contain $ sign")
//
//		verifyPrometheusVolumes(t, clientset)
//		verifyPrometheusConfigMap(t, clientset)
//
//		manualTAConfig := getManualTargetAllocatorConfig()
//		verifyTargetAllocatorConfig(t, clientset, manualTAConfig)
//	})
//}
//
//func testMultipleCollectors(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_multiple_collectors", func(t *testing.T) {
//		// Test case 10
//		collectorCount := getCollectorCount(clientset)
//		require.Greater(t, collectorCount, 1, "Target Allocator should run with multiple collectors")
//	})
//}
//
//func testReconciler(t *testing.T, clientset *kubernetes.Clientset) {
//	t.Run("verify_reconciler", func(t *testing.T) {
//		// Test cases 14, 15
//		servicePort := getServicePort(clientset, "target-allocator-service")
//		require.Equal(t, 8080, servicePort, "Service port should be 8080")
//
//		initialResourceCount := countTargetAllocatorResources(clientset)
//		modifyTargetAllocatorResources(clientset)
//		time.Sleep(time.Second * 30) // Wait for reconciler
//		finalResourceCount := countTargetAllocatorResources(clientset)
//		require.Equal(t, initialResourceCount, finalResourceCount, "Reconciler should add and remove resources as expected")
//	})
//}
//
//// Helper functions (these would need to be implemented)
//func setupKubernetesClientset(t *testing.T) *kubernetes.Clientset {
//	// Implementation to set up and return a Kubernetes clientset
//	return nil
//}
//
//func isTargetAllocatorEnabled(clientset *kubernetes.Clientset) bool {
//	// Implementation
//	return false
//}
//
//func hasValidPrometheusConfig(clientset *kubernetes.Clientset) bool {
//	// Implementation
//	return false
//}
//t
//func isStatefulSetMode(clientset *kubernetes.Clientset) bool {
//	// Implementation
//	return false
//}

// ... (implement other helper functions)
