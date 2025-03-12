package container_insights

import (
	"flag"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
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
		testApplicationSignalsMetrics,
	},
}
var testTracesRegistry = map[string][]func(*testing.T){
	".": {
		testApplicationSignalsTraces,
	},
}
var testResourcesRegistry = []func(*testing.T, *kubernetes.Clientset){
	testAgentResources,
	testSampleAppDeployment,
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
	env.SampleApp = filepath.Join("resources", "appsignals_sample_app.yaml")
	// Configure AWS clients and create K8s resources
	if err := e2e.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
	}

	k8sCtl = utils.NewK8CtlManager(env)
	k8sCtl.Execute([]string{
		"adm",
		"policy",
		"add-scc-to-user",
		"privileged",
		"-z",
		"petclinic-sa",
		"-n",
		"appsig-sample",
	})

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
		t.Run("Traces", func(t *testing.T) {
			testTraces(t)
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
	time.Sleep(10 * time.Minute) // we are checking a period of 10 minutes

	for _, testFunc := range tests {
		testFunc(t)
	}
}
func testTraces(t *testing.T) {
	configFile := filepath.Base(env.AgentConfig)
	tests := testTracesRegistry[configFile]

	fmt.Printf("waiting for traces to propagate for %f minutes ...\n", e2e.Wait.Minutes())
	//time.Sleep(10 * time.Minute) // we are checking a period of 10 minutes

	for _, testFunc := range tests {
		testFunc(t)
	}
}

//------------------------------------------------------------------------------
// Resource Test Functions
//------------------------------------------------------------------------------

func testAgentResources(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_agent_resources", func(t *testing.T) {
		e2e.VerifyAgentResources(t, clientset, "application_signals")
	})
}
func testSampleAppDeployment(t *testing.T, clientset *kubernetes.Clientset) {
	t.Run("verify_sample_app_deployment", func(t *testing.T) {
		e2e.GetPodList(t, clientset, "appsig-sample", "petclinic-app")
	})
}

//------------------------------------------------------------------------------
// Metric Test Functions
//------------------------------------------------------------------------------

func testApplicationSignalsMetrics(t *testing.T) {
	metricsToFetch := metric.AppSignalsMetricNames
	testResults := make([]status.TestResult, len(metricsToFetch))
	instructions := GetInstructionsFromEnv(env)
	dimFactory := dimension.GetDimensionFactory(*env)
	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		for j := 0; j < TestRetryCount; j++ {
			testResult = metric.ValidateAppSignalsMetric(dimFactory, "ApplicationSignals", metricName, instructions)
			if testResult.Status == status.SUCCESSFUL {
				break
			}
			time.Sleep(30 * time.Second)
		}
		testResults[i] = testResult
	}
	for _, result := range testResults {

		if result.Status == status.FAILED {
			t.Errorf("%s test group failed\n", result.Name)
		}
	}
}
func testApplicationSignalsTraces(t *testing.T) {
	xrayFilter := fmt.Sprintf("Annotation[aws.local.environment] = \"%s\"", GetMetricEnvironment(env))

	timeNow := time.Now().UTC()
	traceIds, err := awsservice.GetTraceIDs(timeNow.Add(-10*time.Minute), timeNow, xrayFilter)
	if err != nil {
		require.NoError(t, err, "error getting trace ids: %v", err)
	} else {
		fmt.Printf("Trace IDs: %v\n", traceIds)
		require.Greater(t, len(traceIds), 0)
	}
}

func GetInstructionsFromEnv(env *environment.MetaData) []dimension.Instruction {
	instructions := []dimension.Instruction{
		{
			Key:   "Service",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("petclinic-1")},
		},
	}

	switch env.ComputeType {
	case computetype.EKS:
		instructions = append(instructions, []dimension.Instruction{
			{
				Key:   "HostedIn.EKS.Cluster",
				Value: dimension.UnknownDimensionValue(),
			},
			{
				Key:   "HostedIn.K8s.Namespace",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("appsig-sample")},
			},
		}...)
		break
	case computetype.ROSA:
		instructions = append(instructions, []dimension.Instruction{
			{
				Key:   "Environment",
				Value: dimension.ExpectedDimensionValue{Value: aws.String(GetMetricEnvironment(env))},
			},
		}...)
		break
	default:
		instructions = append(instructions, dimension.Instruction{
			Key:   "HostedIn.Environment",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("Generic")},
		})
		break
	}
	return instructions
}

func GetMetricEnvironment(env *environment.MetaData) string {
	return fmt.Sprintf("k8s:%s/appsig-sample", env.EKSClusterName)
}
