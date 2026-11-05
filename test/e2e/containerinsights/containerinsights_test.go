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

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksinstallationtype"
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
// End-to-end test for OpenTelemetry Container Insights as customers install it:
// the amazon-cloudwatch-observability Helm chart (or the EKS add-on) with
// otelContainerInsights.enabled. The chart renders the collector config into each
// AmazonCloudWatchAgent CR's otelConfig, so the agent runs it unchanged:
//
//   cloudwatch-agent                 -> DaemonSet  (per-node metrics from
//                   cadvisor/kubeletstats/node_exporter + node and application logs)
//   cloudwatch-agent-cluster-scraper -> Deployment (cluster-wide metrics from the
//                   apiserver, kube-state-metrics and the KEDA/Karpenter solutions)
//
// Metrics are exported to the CloudWatch OTLP metrics endpoint
// (monitoring.<region>.amazonaws.com/v1/metrics) and validated via the PromQL
// query client. Node/application logs are exported to CloudWatch Logs and
// validated via the CloudWatch Logs API. Each run creates its own cluster, so
// telemetry is isolated by k8s.cluster.name.

const (
	agentNamespace        = "amazon-cloudwatch"
	clusterScraperCRName  = "cloudwatch-agent-cluster-scraper"
	nodeCRName            = "cloudwatch-agent"
	kedaKarpenterManifest = "resources/keda_karpenter.yaml"
)

var (
	env         *environment.MetaData
	clusterName string
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

// kedaMetrics are emitted by the cluster-scraper KEDA solution pipeline
// (otelContainerInsights.solutions.keda), scraped from the stub keda-operator in the keda namespace.
var kedaMetrics = []string{
	"keda_scaler_active",
	"keda_scaledobject_paused",
}

// karpenterMetrics are emitted by the cluster-scraper Karpenter solution pipeline
// (otelContainerInsights.solutions.karpenter), scraped from the stub karpenter in kube-system.
var karpenterMetrics = []string{
	"karpenter_nodes_total",
	"karpenter_pods_state",
}

// ciLogGroups are the CloudWatch Logs groups produced by the node logs
// pipelines (otelContainerInsights.logs.enabled, on by default).
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

	// Installs the chart (or waits for the add-on) and waits for the operator.
	if err := e2e.InitializeEnvironment(env); err != nil {
		fmt.Printf("Failed to initialize environment: %v\n", err)
		os.Exit(1)
	}

	// The EKS add-on pins released images, and its image patching only covers the
	// node CR, so point the cluster-scraper CR at the build under test as well.
	if env.EKSInstallationType == eksinstallationtype.EKS_ADDON {
		if err := patchClusterScraperImage(env); err != nil {
			fmt.Printf("Failed to patch cluster-scraper image: %v\n", err)
			os.Exit(1)
		}
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

// patchResourceWithRetry wraps K8CtlManager.PatchResource with a bounded retry so the
// add-on path tolerates add-on-activation timing: the target CRs may not exist yet when
// this first runs, and a single-shot patch would fail with "not found" and abort the suite.
func patchResourceWithRetry(k8ctl *utils.K8CtlManager, resourceType, resourceName, namespace string, patchType utils.PatchType, patchData string) error {
	const (
		patchRetryTimeout  = 2 * time.Minute
		patchRetryInterval = 5 * time.Second
	)
	deadline := time.Now().Add(patchRetryTimeout)
	for attempt := 1; ; attempt++ {
		err := k8ctl.PatchResource(resourceType, resourceName, namespace, patchType, patchData)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("patching %s/%s failed after %s (%d attempts): %w",
				resourceType, resourceName, patchRetryTimeout, attempt, err)
		}
		fmt.Printf("Patch %s/%s attempt %d failed, retrying in %s: %v\n",
			resourceType, resourceName, attempt, patchRetryInterval, err)
		time.Sleep(patchRetryInterval)
	}
}

// patchClusterScraperImage sets the cluster-scraper CR image to the build under test.
// The Helm chart wires this through agent.image values; the add-on image patching only
// covers the node CR, so we patch the cluster-scraper here for parity.
func patchClusterScraperImage(env *environment.MetaData) error {
	image := fmt.Sprintf("%s/%s:%s",
		env.CloudwatchAgentRepositoryURL,
		env.CloudwatchAgentRepository,
		env.CloudwatchAgentTag)

	patch, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{"image": image},
	})
	if err != nil {
		return fmt.Errorf("marshaling cluster-scraper image patch: %w", err)
	}

	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(resolveClusterName(env)); err != nil {
		return err
	}
	return patchResourceWithRetry(
		k8ctl,
		"amazoncloudwatchagent",
		clusterScraperCRName,
		agentNamespace,
		utils.PatchTypeMerge,
		string(patch),
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
// Deployment were created by the operator from the chart's CRs.
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

// validateMetrics asserts every metric name is present in CloudWatch for THIS run's
// cluster, using the shared otlp validator.
func validateMetrics(t *testing.T, metrics []string) {
	labels := map[string]string{"@resource.k8s.cluster.name": clusterName}
	res := otlpvalidation.ValidateOtlpMetricsWithLabels(t.Name(), env.Region, metrics, labels)
	for _, r := range res.TestResults {
		r := r
		t.Run(r.Name, func(t *testing.T) {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric validation %s failed (cluster=%s): %v", r.Name, clusterName, r.Reason)
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

			// The log group is scoped to this run's cluster; require events in our time window.
			err := awsservice.ValidateLogs(logGroup, streams[0], &since, &until, awsservice.AssertLogsNotEmpty())
			require.NoError(t, err, "validating logs in %s/%s", logGroup, streams[0])
		})
	}
}
