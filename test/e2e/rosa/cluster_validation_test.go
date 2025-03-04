package rosa

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/stretchr/testify/require"
)

//------------------------------------------------------------------------------
// Variables
//------------------------------------------------------------------------------

var (
	env    *environment.MetaData
	k8sCtl *utils.K8CtlManager
)

//------------------------------------------------------------------------------
// Test Registry Maps
//------------------------------------------------------------------------------

var testPrerequisitesRegistry = []func(*testing.T){
	testRosaCliInstalled,
	testOcCliInstalled,
}

var testClusterRegistry = []func(*testing.T){
	testLoginStatus,
	testNodeStatus,
	testCoreServices,
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

	os.Exit(m.Run())
}

//------------------------------------------------------------------------------
// Main Test Functions
//------------------------------------------------------------------------------

func TestRosaCluster(t *testing.T) {
	t.Run("Prerequisites", func(t *testing.T) {
		testPrerequisites(t)
	})

	// Don't run cluster tests if prerequisites fail
	if !t.Failed() {
		t.Run("Cluster Tests", func(t *testing.T) {
			testCluster(t)
		})
	}
}

func testPrerequisites(t *testing.T) {
	for _, testFunc := range testPrerequisitesRegistry {
		testFunc(t)
	}
}

func testCluster(t *testing.T) {
	for _, testFunc := range testClusterRegistry {
		testFunc(t)
	}
}

//------------------------------------------------------------------------------
// Prerequisite Test Functions
//------------------------------------------------------------------------------

func testRosaCliInstalled(t *testing.T) {
	cmd := exec.Command("rosa", "version")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "ROSA CLI not found: %v\n%s", err, string(output))
	version := strings.TrimSpace(string(output))
	require.NotNil(t, version)
	t.Logf("ROSA CLI version: %s", version)
}

func testOcCliInstalled(t *testing.T) {
	cmd := exec.Command("oc", "version")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "OC CLI not found: %v\n%s", err, string(output))
	version := strings.TrimSpace(string(output))
	require.NotNil(t, version)
	t.Logf("OpenShift CLI version: %s", version)
}

//------------------------------------------------------------------------------
// Cluster Test Functions
//------------------------------------------------------------------------------

func testLoginStatus(t *testing.T) {
	cmd := exec.Command("oc", "whoami")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Not logged into the cluster: %v\n%s", err, output)
		return
	}

	user := strings.TrimSpace(string(output))
	t.Logf("Logged in as: %s", user)
}

func testNodeStatus(t *testing.T) {
	cmd := exec.Command("oc", "get", "nodes", "-o", "json")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get nodes: %v\n%s", err, output)
	}

	var nodes struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &nodes); err != nil {
		t.Fatalf("Failed to parse node JSON: %v", err)
	}

	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				t.Errorf("Node %s is not ready", node.Metadata.Name)
			}
		}
	}
}

func testCoreServices(t *testing.T) {
	coreNamespaces := []string{"openshift-apiserver", "openshift-controller-manager", "openshift-etcd"}

	for _, namespace := range coreNamespaces {
		t.Run(namespace, func(t *testing.T) {
			cmd := exec.Command("oc", "get", "pods", "-n", namespace, "-o", "json")
			cmd.Env = os.Environ()
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to get pods in %s: %v\n%s", namespace, err, output)
			}

			var pods struct {
				Items []struct {
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Status struct {
						Phase string `json:"phase"`
					} `json:"status"`
				} `json:"items"`
			}

			if err := json.Unmarshal(output, &pods); err != nil {
				t.Fatalf("Failed to parse pod JSON: %v", err)
			}

			for _, pod := range pods.Items {
				if pod.Status.Phase != "Running" {
					t.Errorf("Pod %s in %s is not running (status: %s)",
						pod.Metadata.Name, namespace, pod.Status.Phase)
				}
			}
		})
	}
}
