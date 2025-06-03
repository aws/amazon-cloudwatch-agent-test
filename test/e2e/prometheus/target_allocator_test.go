// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	//"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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
	testTargetAllocatorBasics,
	testTargetAllocatorServer,
	testTargetAllocatorMetrics,
	testTargetAllocatorScaling,
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
		e2e.VerifyAgentResourcesStatefulSet(t, clientset, "prometheus")
	})
}

func testTargetAllocatorBasics(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_target_allocator_resources", func(t *testing.T) {
		fmt.Println("Verifying Target Allocator resources...")

		// Check ConfigMap
		configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator ConfigMap")
		require.NotNil(t, configMap, "Target Allocator ConfigMap not found")

		// Verify ConfigMap content
		config, exists := configMap.Data["config.yaml"]
		require.True(t, exists, "config.yaml not found in ConfigMap")
		require.Contains(t, config, "scrape_configs", "ConfigMap should contain scrape_configs")

		// Check Service
		service, err := clientset.CoreV1().Services("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator Service")
		require.NotNil(t, service, "Target Allocator Service not found")
		require.Equal(t, int32(8080), service.Spec.Ports[0].Port, "Target Allocator service port should be 8080")

		// Check ServiceAccount
		sa, err := clientset.CoreV1().ServiceAccounts("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator ServiceAccount")
		require.NotNil(t, sa, "Target Allocator ServiceAccount not found")

		// Check Deployment
		deployment, err := clientset.AppsV1().Deployments("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator Deployment")
		require.NotNil(t, deployment, "Target Allocator Deployment not found")

		// Check Deployment replicas
		require.Equal(t, int32(1), *deployment.Spec.Replicas, "Target Allocator should have 1 replica")

		// Check Deployment containers
		require.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers), "Target Allocator should have 1 container")
		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, "cloudwatch-agent-target-allocator", container.Name, "Container name should be cloudwatch-agent-target-allocator")

		// Check container image
		require.Contains(t, container.Image, "cloudwatch-agent-target-allocator", "Container image should be cloudwatch-agent-target-allocator")

		// Check container ports
		require.Equal(t, 1, len(container.Ports), "Container should expose 1 port")
		require.Equal(t, int32(8080), container.Ports[0].ContainerPort, "Container port should be 8080")
	})
}

func testTargetAllocatorServer(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_target_allocator_server", func(t *testing.T) {
		fmt.Println("Verifying Target Allocator server endpoints...")

		// Setup port-forward
		portForward, err := setupPortForward(clientset, "amazon-cloudwatch", "cloudwatch-agent-target-allocator", 8080)
		require.NoError(t, err, "Failed to setup port-forward")
		defer portForward.Process.Kill()

		endpoint := "http://localhost:8080"

		// Test scraper config endpoint
		config := getScraperConfig(endpoint)
		require.NotEmpty(t, config, "Scraper config should not be empty")
		require.Contains(t, config, "scrape_configs", "Scraper config should contain scrape_configs")

		// Parse and validate scraper config
		var scraperConfig map[string]interface{}
		err = json.Unmarshal([]byte(config), &scraperConfig)
		require.NoError(t, err, "Failed to parse scraper config")
		require.Contains(t, scraperConfig, "global", "Scraper config should contain global settings")
		require.Contains(t, scraperConfig, "scrape_configs", "Scraper config should contain scrape_configs")

		// Test jobs endpoint
		jobs := getJobs(endpoint)
		require.NotEmpty(t, jobs, "Jobs list should not be empty")

		// Parse and validate jobs
		var jobsList []string
		err = json.Unmarshal([]byte(jobs), &jobsList)
		require.NoError(t, err, "Failed to parse jobs list")
		require.True(t, len(jobsList) > 0, "Jobs list should not be empty")

		// Test job per collector endpoint
		jobPerCollector := getJobPerCollector(endpoint, jobsList[0])
		require.NotEmpty(t, jobPerCollector, "Job per collector should not be empty")

		// Parse and validate job per collector
		var jobConfig map[string]interface{}
		err = json.Unmarshal([]byte(jobPerCollector), &jobConfig)
		require.NoError(t, err, "Failed to parse job config")
		require.Contains(t, jobConfig, "job_name", "Job config should contain job_name")
		require.Contains(t, jobConfig, "scrape_interval", "Job config should contain scrape_interval")

		// Test health endpoints
		livezStatus := getEndpointStatus(endpoint + "/livez")
		readyzStatus := getEndpointStatus(endpoint + "/readyz")
		require.Equal(t, 200, livezStatus, "Livez endpoint should return 200")
		require.Equal(t, 200, readyzStatus, "Readyz endpoint should return 200")
	})
}

func testTargetAllocatorMetrics(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_target_allocator_metrics", func(t *testing.T) {
		fmt.Println("Verifying Target Allocator metrics...")

		// Setup port-forward
		portForward, err := setupPortForward(clientset, "amazon-cloudwatch", "cloudwatch-agent-target-allocator", 8080)
		require.NoError(t, err, "Failed to setup port-forward")
		defer portForward.Process.Kill()

		endpoint := "http://localhost:8080"

		// Get metrics
		metrics := getMetrics(endpoint + "/metrics")
		require.NotEmpty(t, metrics, "Metrics should not be empty")

		// Check for specific metrics
		requiredMetrics := []string{
			"cloudwatch_agent_target_allocator_allocations_total",
			"cloudwatch_agent_target_allocator_targets",
			"cloudwatch_agent_target_allocator_scrapes_total",
			"cloudwatch_agent_target_allocator_scrape_duration_seconds",
		}

		for _, metric := range requiredMetrics {
			require.Contains(t, metrics, metric, fmt.Sprintf("Metrics should contain %s", metric))
		}
	})
}

func testTargetAllocatorScaling(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_target_allocator_scaling", func(t *testing.T) {
		fmt.Println("Verifying Target Allocator scaling...")

		// Get current replica count
		deployment, err := clientset.AppsV1().Deployments("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator Deployment")
		initialReplicas := *deployment.Spec.Replicas

		// Scale up
		newReplicas := initialReplicas + 1
		deployment.Spec.Replicas = &newReplicas
		_, err = clientset.AppsV1().Deployments("amazon-cloudwatch").Update(
			context.TODO(),
			deployment,
			metav1.UpdateOptions{},
		)
		require.NoError(t, err, "Error scaling up Target Allocator")

		// Wait for scaling
		err = waitForDeploymentReady(clientset, "amazon-cloudwatch", "cloudwatch-agent-target-allocator", 2*time.Minute)
		require.NoError(t, err, "Target Allocator failed to scale up")

		// Verify new replica count
		deployment, err = clientset.AppsV1().Deployments("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator Deployment after scaling")
		require.Equal(t, newReplicas, *deployment.Spec.Replicas, "Target Allocator should have scaled up")

		// Scale back down
		deployment.Spec.Replicas = &initialReplicas
		_, err = clientset.AppsV1().Deployments("amazon-cloudwatch").Update(
			context.TODO(),
			deployment,
			metav1.UpdateOptions{},
		)
		require.NoError(t, err, "Error scaling down Target Allocator")

		// Wait for scaling down
		err = waitForDeploymentReady(clientset, "amazon-cloudwatch", "cloudwatch-agent-target-allocator", 2*time.Minute)
		require.NoError(t, err, "Target Allocator failed to scale down")

		// Verify original replica count
		deployment, err = clientset.AppsV1().Deployments("amazon-cloudwatch").Get(
			context.TODO(),
			"cloudwatch-agent-target-allocator",
			metav1.GetOptions{},
		)
		require.NoError(t, err, "Error getting Target Allocator Deployment after scaling down")
		require.Equal(t, initialReplicas, *deployment.Spec.Replicas, "Target Allocator should have scaled back down")
	})
}

// Helper function to wait for deployment to be ready
func waitForDeploymentReady(clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas, nil
	})
}

// Helper function to get metrics
func getMetrics(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func setupPortForward(clientset *kubernetes.Clientset, namespace, service string, port int) (*exec.Cmd, error) {
	cmd := exec.Command("kubectl", "port-forward",
		fmt.Sprintf("service/%s", service),
		fmt.Sprintf("%d:%d", port, port),
		"-n", namespace)

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	// Wait for port-forward to be ready
	time.Sleep(2 * time.Second)
	return cmd, nil
}

func getScraperConfig(endpoint string) string {
	resp, err := http.Get(endpoint + "/config")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func getJobs(endpoint string) string {
	resp, err := http.Get(endpoint + "/jobs")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func getJobPerCollector(endpoint, collectorID string) string {
	resp, err := http.Get(fmt.Sprintf("%s/jobs/%s", endpoint, collectorID))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func getEndpointStatus(url string) int {
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
