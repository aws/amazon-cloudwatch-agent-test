// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eks_resources

import (
	_ "embed"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

var (
	//go:embed test_schemas/cluster.json
	eksClusterSchema string
	//go:embed test_schemas/cluster_daemonset.json
	eksClusterDaemonsetSchema string
	//go:embed test_schemas/cluster_deployment.json
	eksClusterDeploymentSchema string
	//go:embed test_schemas/cluster_namespace.json
	eksClusterNamespaceSchema string
	//go:embed test_schemas/cluster_service.json
	eksClusterServiceSchema string
	//go:embed test_schemas/container.json
	eksContainerSchema string
	//go:embed test_schemas/container_fs.json
	eksContainerFSSchema string
	//go:embed test_schemas/control_plane.json
	eksControlPlaneSchema string
	//go:embed test_schemas/node.json
	eksNodeSchema string
	//go:embed test_schemas/node_disk_io.json
	eksNodeDiskIOSchema string
	//go:embed test_schemas/node_fs.json
	eksNodeFSSchema string
	//go:embed test_schemas/node_net.json
	eksNodeNetSchema string
	//go:embed test_schemas/pod.json
	eksPodSchema string
	//go:embed test_schemas/pod_net.json
	eksPodNetSchema string

	EksClusterValidationMap = map[string]string{
		"Cluster":           eksClusterSchema,
		"ClusterDaemonset":  eksClusterDaemonsetSchema,
		"ClusterDeployment": eksClusterDeploymentSchema,
		"ClusterNamespace":  eksClusterNamespaceSchema,
		"ClusterService":    eksClusterServiceSchema,
		"Container":         eksContainerSchema,
		"ContainerFS":       eksContainerFSSchema,
		"ControlPlane":      eksControlPlaneSchema,
		"Node":              eksNodeSchema,
		"NodeDiskIO":        eksNodeDiskIOSchema,
		"NodeFS":            eksNodeFSSchema,
		"NodeNet":           eksNodeNetSchema,
		"Pod":               eksPodSchema,
		"PodNet":            eksPodNetSchema,
	}
)

type DimensionsToMetricsMap struct {
	Dims    []types.Dimension
	Metrics []string
}

var DimensionStringToMetricsMap = map[string][]string{
	"ClusterName": {"pod_cpu_utilization", "pod_memory_utilization", "pod_network_rx_bytes", "pod_network_tx_bytes",
		"pod_cpu_utilization_over_pod_limit", "pod_memory_utilization_over_pod_limit", "pod_interface_network_rx_dropped", "pod_interface_network_tx_dropped", "namespace_number_of_running_pods"},
}
