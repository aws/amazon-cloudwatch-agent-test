package rosa

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var clusterName string

func init() {
	//flag.StringVar(&clusterName, "cluster-name", "", "Name of the cluster to test")
}

func TestRosaCluster(t *testing.T) {
	t.Logf("Testing cluster: %s", clusterName)

	// Track prerequisite failures
	prerequisitesPassed := true

	t.Run("ROSA CLI Installation", func(t *testing.T) {
		if !testRosaCliInstalled(t) {
			prerequisitesPassed = false
		}
	})
	t.Run("OC CLI Installation", func(t *testing.T) {
		if !testOcCliInstalled(t) {
			prerequisitesPassed = false
		}
	})

	if !prerequisitesPassed {
		t.Log("Skipping cluster tests due to failed prerequisites")
		return
	}

	t.Run("Login Status", testLoginStatus)
	t.Run("Node Status", testNodeStatus)
	t.Run("Core Services", testCoreServices)
}

func testRosaCliInstalled(t *testing.T) bool {
	cmd := exec.Command("rosa", "version")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("ROSA CLI not found: %v\n%s", err, output)
		return false
	}

	version := strings.TrimSpace(string(output))
	t.Logf("ROSA CLI version: %s", version)
	return true
}

func testOcCliInstalled(t *testing.T) bool {
	cmd := exec.Command("oc", "version")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("OpenShift CLI (oc) not found: %v\n%s", err, output)
		return false
	}

	version := strings.TrimSpace(string(output))
	t.Logf("OpenShift CLI version: %s", version)
	return true
}

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
					t.Errorf("Pod %s in %s is not running (status: %s)", pod.Metadata.Name, namespace, pod.Status.Phase)
				}
			}
		})
	}
}
