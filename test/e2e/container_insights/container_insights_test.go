package container_insights

import (
	"flag"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ------------------------------------------------------------------------------
// Variables
// ------------------------------------------------------------------------------
const (
	TestRetryCount = 3
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
		testContainerInsightsMetrics,
	},
}

var testResourcesRegistry = []func(*testing.T, *kubernetes.Clientset){
	testAgentResources,
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
	time.Sleep(e2e.WaitForResourceCreation)
	for _, testFunc := range tests {
		testFunc(t, clientset)
	}
}

func testMetrics(t *testing.T) {
	configFile := filepath.Base(env.AgentConfig)
	tests := testMetricsRegistry[configFile]

	fmt.Printf("waiting for metrics to propagate for %f minutes ...\n", e2e.Wait.Minutes())
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
		e2e.VerifyAgentResources(t, clientset, "container_insights")
	})
}

//------------------------------------------------------------------------------
// Metric Test Functions
//------------------------------------------------------------------------------

func testContainerInsightsMetrics(t *testing.T) {
	var testResults []status.TestResult

	expectedMetrics := eks_resources.GetExpectedDimsToMetrics(env)
	//Merge enhancedMetrics into combinedMetrics
	for key, value := range eks_resources.GetExpectedDimsToMetricsForEnhanced(env) {
		if existingValue, exists := expectedMetrics[key]; exists {
			// If the key already exists, append the new metrics to the existing slice
			expectedMetrics[key] = append(existingValue, value...)
		} else {
			// If the key doesn't exist, add it to the combined map
			expectedMetrics[key] = value
		}
	}
	for j := 0; j < TestRetryCount; j++ {
		testResults = append(testResults, metric.ValidateMetrics(env, "", expectedMetrics)...)
		for _, result := range testResults {

			if result.Status == status.FAILED {
				t.Errorf("%s test group failed\n", result.Name)
			}
		}
		time.Sleep(1 * time.Minute)
	}
}
