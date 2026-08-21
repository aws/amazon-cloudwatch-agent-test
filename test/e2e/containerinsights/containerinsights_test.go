// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//------------------------------------------------------------------------------
// Overview
//------------------------------------------------------------------------------
//
// End-to-end test for the OpenTelemetry Container Insights pipeline driven
// directly by the CloudWatch Agent JSON config (opentelemetry.collect.
// container_insights). Two agent configs run concurrently in the cluster,
// each on its own workload / AmazonCloudWatchAgent CR:
//
//   role=node    -> DaemonSet  "cloudwatch-agent"                 (per-node
//                   metrics from cadvisor/kubeletstats/node_exporter + node
//                   and application logs)
//   role=cluster -> Deployment "cloudwatch-agent-cluster-scraper" (cluster-wide
//                   metrics from the apiserver and kube-state-metrics)
//
// The agent translator builds the OTEL pipelines from the JSON config. Metrics
// are exported to the CloudWatch OTLP metrics endpoint
// (monitoring.<region>.amazonaws.com/v1/metrics) and validated via the PromQL
// query client. Node/application logs are exported to CloudWatch Logs and
// validated via the CloudWatch Logs API.

const (
	agentNamespace        = "amazon-cloudwatch"
	clusterScraperCRName  = "cloudwatch-agent-cluster-scraper"
	nodeCRName            = "cloudwatch-agent"
	clusterConfigPath     = "resources/cwagent_configs_helm_chart/ci_cluster.json"
	kedaKarpenterManifest = "resources/keda_karpenter.yaml"
	testRunIDAttribute    = "test.run.id"
)

var (
	env         *environment.MetaData
	clusterName string
	// testRunID is a per-invocation identifier stamped onto every metric/log via
	// opentelemetry.resource_attributes.
	testRunID string
)

var nodeMetrics = []string{
	// cadvisor
	"container_cpu_usage_seconds_total",
	"container_memory_working_set_bytes",
	// kubeletstats
	"k8s.node.cpu.usage",
	"k8s.node.memory.working_set",
	"k8s.pod.cpu.usage",
	// node_exporter
	"node_cpu_seconds_total",
	"node_memory_MemAvailable_bytes",
}

var clusterMetrics = []string{
	// apiserver / control plane
	"apiserver_request_total",
	// kube-state-metrics
	"kube_node_info",
	"kube_pod_info",
}

// kedaMetrics are emitted by the role=cluster KEDA solution pipeline
// (solutions.keda.enabled), scraped from the keda-operator in the keda namespace.
var kedaMetrics = []string{
	"keda_scaler_active",
	"keda_scaledobject_paused",
}

// karpenterMetrics are emitted by the role=cluster Karpenter solution pipeline
// (solutions.karpenter.enabled), scraped from karpenter in the karpenter namespace.
var karpenterMetrics = []string{
	"karpenter_nodes_total",
	"karpenter_pods_state",
}

// ciLogGroups are the CloudWatch Logs groups produced by the role=node logs
// pipelines when logs.enabled.
var ciLogGroups = []string{
	"/aws/otel/containerinsights/%s/application",
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestMain(m *testing.M) {
	flag.Parse()

	// Skip when invoked with the sentinel run filter.
	if flag.Lookup("test.run") != nil && flag.Lookup("test.run").Value.String() == "NO_MATCH" {
		os.Exit(0)
	}

	env = environment.GetEnvironmentMetaData()

	// terraform destroy path: tear resources down and exit.
	if env.Destroy {
		if err := deleteKedaKarpenterStubs(env); err != nil {
			fmt.Printf("Failed to delete keda/karpenter stubs: %v\n", err)
		}
		if err := e2e.DestroyResources(env); err != nil {
			fmt.Printf("Failed to delete kubernetes resources: %v\n", err)
		}
		os.Exit(0)
	}

	// Per-run identifier stamped onto every metric/log via opentelemetry.resource_attributes
	testRunID = "test-run-" + uuid.NewString()[:8]
	if env.AgentConfig != "" {
		name := resolveClusterName(env)
		injected, err := injectTestID(env.AgentConfig, testRunID, name)
		if err != nil {
			fmt.Printf("Failed to inject test.run.id into node config: %v\n", err)
			os.Exit(1)
		}
		env.AgentConfig = injected
	}
	fmt.Printf("test.run.id=%s\n", testRunID)

	// Applies the node config (env.AgentConfig, role=node) to the primary
	// cloudwatch-agent CR via helm/addon and waits for the operator.
	if err := e2e.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
	}

	// Clear pre-rendered otelConfig on the node CR so agent translates json config
	if err := clearOtelConfig(env, nodeCRName); err != nil {
		fmt.Printf("Failed to clear node otelConfig: %v\n", err)
		os.Exit(1)
	}

	// Apply the cluster config (role=cluster) to the cluster-scraper CR.
	if err := applyClusterConfig(env); err != nil {
		fmt.Printf("Failed to apply cluster-scraper config: %v\n", err)
		os.Exit(1)
	}

	// Deploy the KEDA/Karpenter stub emitters so the solutions pipelines have
	// pods to scrape.
	if err := applyKedaKarpenterStubs(env); err != nil {
		fmt.Printf("Failed to apply keda/karpenter stubs: %v\n", err)
		os.Exit(1)
	}

	region := env.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	clusterName = resolveClusterName(env)
	if region == "" || clusterName == "" {
		fmt.Fprintf(os.Stderr, "region and cluster name must be set\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// resolveClusterName returns the EKS cluster name, falling back to CLUSTER_NAME.
func resolveClusterName(env *environment.MetaData) string {
	if env.EKSClusterName != "" {
		return env.EKSClusterName
	}
	return os.Getenv("CLUSTER_NAME")
}

// applyClusterConfig patches the cluster-scraper CR with the role=cluster config
// (the node config is applied by InitializeEnvironment).
func applyClusterConfig(env *environment.MetaData) error {
	name := resolveClusterName(env)

	// Read the cluster config from file then set the runtime cluster name and stamp the per-run test.run.id.
	data, err := os.ReadFile(clusterConfigPath)
	if err != nil {
		return fmt.Errorf("reading cluster config %s: %w", clusterConfigPath, err)
	}
	var agentConfig map[string]interface{}
	if err := json.Unmarshal(data, &agentConfig); err != nil {
		return fmt.Errorf("parsing cluster config: %w", err)
	}
	otel, ok := agentConfig["opentelemetry"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("cluster config %s missing opentelemetry block", clusterConfigPath)
	}
	otel["cluster_name"] = name
	otel["resource_attributes"] = map[string]interface{}{testRunIDAttribute: testRunID}

	agentConfigJSON, err := json.Marshal(agentConfig)
	if err != nil {
		return fmt.Errorf("marshaling cluster agent config: %w", err)
	}

	// spec.config is the agent JSON (the agent translates it).
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"config": string(agentConfigJSON),
		},
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling CR patch: %w", err)
	}

	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(name); err != nil {
		return err
	}
	if err := k8ctl.PatchResource(
		"amazoncloudwatchagent",
		clusterScraperCRName,
		agentNamespace,
		utils.PatchTypeMerge,
		string(patchJSON),
	); err != nil {
		return err
	}
	return clearOtelConfig(env, clusterScraperCRName)
}

// clearOtelConfig removes the chart's pre-rendered otelConfig so the agent
// translates spec.config (our JSON) instead.
func clearOtelConfig(env *environment.MetaData, crName string) error {
	name := resolveClusterName(env)
	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(name); err != nil {
		return err
	}
	return k8ctl.PatchResource(
		"amazoncloudwatchagent",
		crName,
		agentNamespace,
		utils.PatchTypeMerge,
		`{"spec":{"otelConfig":""}}`,
	)
}

// applyKedaKarpenterStubs deploys the KEDA/Karpenter stub emitters
// so the cluster-scraper's solutions pipelines have pods to discover and scrape.
func applyKedaKarpenterStubs(env *environment.MetaData) error {
	name := resolveClusterName(env)
	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(name); err != nil {
		return err
	}
	return k8ctl.ApplyResource(kedaKarpenterManifest)
}

// deleteKedaKarpenterStubs removes the stub emitters.
func deleteKedaKarpenterStubs(env *environment.MetaData) error {
	name := resolveClusterName(env)
	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(name); err != nil {
		return err
	}
	return k8ctl.DeleteResource(kedaKarpenterManifest)
}

// injectTestID stamps opentelemetry.resource_attributes[test.run.id]=runID into the
// agent config, writes a temp file, and returns its path (for per-run isolation).
func injectTestID(configPath, runID, clusterName string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}

	// Locate the opentelemetry object (top-level, or nested under agent.config).
	otel, _ := cfg["opentelemetry"].(map[string]interface{})
	if otel == nil {
		if agent, ok := cfg["agent"].(map[string]interface{}); ok {
			if inner, ok := agent["config"].(map[string]interface{}); ok {
				otel, _ = inner["opentelemetry"].(map[string]interface{})
			}
		}
	}
	if otel == nil {
		return "", fmt.Errorf("opentelemetry block not found in %s", configPath)
	}

	// Stamp the real cluster name so node metrics/logs land under the run's
	// cluster instead of the config's placeholder.
	if clusterName != "" {
		otel["cluster_name"] = clusterName
	}

	attrs, _ := otel["resource_attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = map[string]interface{}{}
	}
	attrs[testRunIDAttribute] = runID
	otel["resource_attributes"] = attrs

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := filepath.Join(os.TempDir(), "ci_node_"+runID+".json")
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return "", err
	}
	return tmp, nil
}

func TestContainerInsights(t *testing.T) {
	t.Run("Resources", testResources)

	if t.Failed() {
		return
	}

	fmt.Println("waiting for telemetry to propagate...")
	time.Sleep(e2e.Wait)

	t.Run("NodeMetrics", func(t *testing.T) {
		validateMetrics(t, nodeMetrics)
	})
	t.Run("ClusterMetrics", func(t *testing.T) {
		validateMetrics(t, clusterMetrics)
	})
	t.Run("KedaMetrics", func(t *testing.T) {
		validateMetrics(t, kedaMetrics)
	})
	t.Run("KarpenterMetrics", func(t *testing.T) {
		validateMetrics(t, karpenterMetrics)
	})
	t.Run("NodeLogs", testNodeLogs)
}

// testResources verifies that both the node DaemonSet and the cluster-scraper
// Deployment were created by the operator from the applied configs.
func testResources(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err, "building kubeconfig")
	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "creating clientset")

	time.Sleep(e2e.WaitForResourceCreation)

	t.Run("node_daemonset", func(t *testing.T) {
		ctx := context.Background()
		ds, err := clientset.AppsV1().DaemonSets(agentNamespace).Get(ctx, nodeCRName, metav1.GetOptions{})
		require.NoError(t, err, "getting node DaemonSet")
		require.NotNil(t, ds, "node DaemonSet not found")
	})

	t.Run("cluster_scraper_deployment", func(t *testing.T) {
		ctx := context.Background()
		dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(ctx, clusterScraperCRName, metav1.GetOptions{})
		require.NoError(t, err, "getting cluster-scraper Deployment")
		require.NotNil(t, dep, "cluster-scraper Deployment not found")
	})
}

// validateMetrics asserts every metric name is present in CloudWatch for THIS run,
// isolated by the test.run.id resource attribute, using the shared otlp validator.
func validateMetrics(t *testing.T, metrics []string) {
	labels := map[string]string{"@resource." + testRunIDAttribute: testRunID}
	res := otlpvalidation.ValidateOtlpMetricsWithLabels(t.Name(), env.Region, metrics, labels)
	for _, r := range res.TestResults {
		r := r
		t.Run(r.Name, func(t *testing.T) {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric validation %s failed (test.run.id=%s): %v", r.Name, testRunID, r.Reason)
		})
	}
}

// testNodeLogs asserts the node and application log groups exist, have streams,
// and contain events.
func testNodeLogs(t *testing.T) {
	since := time.Now().Add(-e2e.Wait)
	until := time.Now()

	for _, groupTmpl := range ciLogGroups {
		logGroup := fmt.Sprintf(groupTmpl, clusterName)
		t.Run(logGroup, func(t *testing.T) {
			streams := awsservice.GetLogStreamNames(logGroup)
			require.NotEmpty(t, streams, "no log streams in %s", logGroup)

			// Validate the events in our time window actually carry this run's test.run.id 
			err := awsservice.ValidateLogs(logGroup, streams[0], &since, &until,
				awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(testRunID)))
			require.NoError(t, err, "validating logs in %s/%s carry %s", logGroup, streams[0], testRunID)
		})
	}
}
