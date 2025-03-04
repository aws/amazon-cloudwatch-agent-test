// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package security

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/gocapability/capability"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

//------------------------------------------------------------------------------
// Variables
//------------------------------------------------------------------------------

const (
	AGENT_NAMESPACE = "amazon-cloudwatch"
	TEST_POD_NAME   = "test-shell"
)

var (
	env         *environment.MetaData
	k8sCtl      *utils.K8CtlManager
	helmManager *utils.HelmManager
)

//------------------------------------------------------------------------------
// Test Registry Maps
//------------------------------------------------------------------------------

var testMetricsRegistry = map[string][]func(*testing.T){
	".": {
		testCapabilities,
		testAgentPIDAccess,
	},
}

var testResourcesRegistry = []func(*testing.T, *kubernetes.Clientset){
	testAgentResources,
	testAgentValidVolumeMountAccess,
	testAgentInvalidVolumeMountAccess,
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
	k8sCtl = utils.NewK8CtlManager(env)

	// Deploy test shell
	testShellManifestPath := filepath.Join("resources", "shell.yaml")
	if err := k8sCtl.ApplyResource(testShellManifestPath); err != nil {
		fmt.Printf("Failed to initialize test shell: %v\n", err)
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
	time.Sleep(1 * time.Minute)

	for _, testFunc := range tests {
		testFunc(t)
	}
}

//------------------------------------------------------------------------------
// Resource Test Functions
//------------------------------------------------------------------------------

func testAgentResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_agent_resources", func(t *testing.T) {
		e2e.VerifyAgentResources(t, clientset, "")
	})
}
func testAgentValidVolumeMountAccess(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_volume_mount_access_valid_resources", func(t *testing.T) {
		testManifestPath := filepath.Join("resources", "valid-volumetypes.yaml")
		err := k8sCtl.ApplyResource(testManifestPath)
		require.NoError(t, err)
		agentPods := e2e.GetPodList(t, clientset, AGENT_NAMESPACE, "volume-test-valid")
		require.Len(t, agentPods.Items, 1, "Pod should be created")
	})
}
func testAgentInvalidVolumeMountAccess(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_volume_mount_access_invalid_resources", func(t *testing.T) {
		testManifestPath := filepath.Join("resources", "invalid-volumetypes.yaml")
		err := k8sCtl.ApplyResource(testManifestPath)
		require.NoError(t, err)
		agentPods := e2e.GetPodList(t, clientset, AGENT_NAMESPACE, "volume-test-invalid")
		require.Len(t, agentPods.Items, 0, "Pods shouldn't be created")
	})

}

//------------------------------------------------------------------------------
// Metric Test Functions
//------------------------------------------------------------------------------

func testCapabilities(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err, "Error building kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "Error creating clientset")
	agentPods := e2e.GetPodList(t, clientset, AGENT_NAMESPACE, TEST_POD_NAME)
	require.Greater(t, len(agentPods.Items), 0, "No pods found")

	testPod := agentPods.Items[0]
	cmd := []string{"capsh", "--current"}
	result, err := k8sCtl.ExecuteCommand(testPod.Name, AGENT_NAMESPACE, cmd)
	if err != nil {
		require.NoError(t, err, "Failed to execute capsh command")
	}

	t.Log(result)
	allowedCaps, disallowedCaps := separateCurrentsCapabilities(result)
	require.Greater(t, len(allowedCaps), 0, "No capabilities found")
	require.Greater(t, len(disallowedCaps), 0, "No disallowed capabilities found")

	agentCapabilities := []capability.Cap{capability.CAP_SYS_ADMIN}

	for _, cap := range getListOfAllLinuxCapabilities() {
		capString := cap.String()
		if slices.Contains(agentCapabilities, cap) {
			require.Contains(t, allowedCaps, capString, "Found missing capabilities")
			continue
		}
		require.Contains(t, disallowedCaps, capString, "Found disallowed capabilities")
	}
}

func testAgentPIDAccess(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err, "Error building kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "Error creating clientset")
	agentPods := e2e.GetPodList(t, clientset, AGENT_NAMESPACE, TEST_POD_NAME)
	require.Greater(t, len(agentPods.Items), 0, "No pods found")

	testPod := agentPods.Items[0]
	cmd := []string{"ps", "x"}
	result, err := k8sCtl.ExecuteCommand(testPod.Name, AGENT_NAMESPACE, cmd)
	if err != nil {
		require.NoError(t, err, "Failed to execute ps command")
	}
	require.NotContains(t, result, "/usr/bin", "No host PIDs found")
	t.Log(result)
}
