// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eks_resources

import (
	_ "embed"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
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
	//go:embed test_schemas/container_gpu.json
	eksContainerGpuSchema string
	//go:embed test_schemas/pod_gpu.json
	eksPodGpuSchema string
	//go:embed test_schemas/node_gpu.json
	eksNodeGpuSchema string
	//go:embed test_schemas/cluster_gpu.json
	eksClusterGpuSchema string
	//go:embed test_schemas/container_neuroncore.json
	eksContainerNeuronCoreSchema string
	//go:embed test_schemas/pod_neuroncore.json
	eksPodNeuronCoreSchema string
	//go:embed test_schemas/node_neuroncore.json
	eksNodeNeuronCoreSchema string
	//go:embed test_schemas/node_neurondevice.json
	eksNodeNeuronDeviceSchema string
	//go:embed test_schemas/node_neuron.json
	eksNodeNeuronSchema string
	//go:embed test_schemas/container_efa.json
	eksContainerEfaSchema string
	//go:embed test_schemas/pod_efa.json
	eksPodEfaSchema string
	//go:embed test_schemas/node_efa.json
	eksNodeEfaSchema string

	EksClusterValidationMap = map[string]string{
		"Cluster":                eksClusterSchema,
		"ClusterDaemonSet":       eksClusterDaemonsetSchema,
		"ClusterDeployment":      eksClusterDeploymentSchema,
		"ClusterNamespace":       eksClusterNamespaceSchema,
		"ClusterService":         eksClusterServiceSchema,
		"Container":              eksContainerSchema,
		"ContainerFS":            eksContainerFSSchema,
		"ControlPlane":           eksControlPlaneSchema,
		"Node":                   eksNodeSchema,
		"NodeDiskIO":             eksNodeDiskIOSchema,
		"NodeFS":                 eksNodeFSSchema,
		"NodeNet":                eksNodeNetSchema,
		"Pod":                    eksPodSchema,
		"PodNet":                 eksPodNetSchema,
		"ContainerGPU":           eksContainerGpuSchema,
		"PodGPU":                 eksPodGpuSchema,
		"NodeGPU":                eksNodeGpuSchema,
		"ClusterGPU":             eksClusterGpuSchema,
		"ContainerAWSNeuronCore": eksContainerNeuronCoreSchema,
		"PodAWSNeuronCore":       eksPodNeuronCoreSchema,
		"NodeAWSNeuronCore":      eksNodeNeuronCoreSchema,
		"NodeAWSNeuronDevice":    eksNodeNeuronDeviceSchema,
		"NodeAWSNeuron":          eksNodeNeuronSchema,
		"ContainerEFA":           eksContainerEfaSchema,
		"PodEFA":                 eksPodEfaSchema,
		"NodeEFA":                eksNodeEfaSchema,
	}

	EksClusterFrequencyValidationMap = map[string]int{
		"NodeAWSNeuronCore":   32,
		"NodeAWSNeuronDevice": 16,
		"NodeAWSNeuron":       1,
	}
)

func GetExpectedDimsToMetrics(env *environment.MetaData) map[string][]string {
	// Hard coded map which lists the expected metrics in each dimension set
	var ExpectedDimsToMetrics = map[string][]string{
		"ClusterName": {
			"pod_number_of_containers",
			"node_status_allocatable_pods",
			"pod_number_of_container_restarts",
			"node_status_condition_unknown",
			"node_number_of_running_pods",
			"pod_container_status_running",
			"node_status_condition_ready",
			"pod_status_running",
			"node_filesystem_utilization",
			"pod_container_status_terminated",
			"pod_status_pending",
			"pod_cpu_utilization",
			"node_filesystem_inodes",
			"node_diskio_io_service_bytes_total",
			"node_status_condition_memory_pressure",
			"container_cpu_utilization",
			"service_number_of_running_pods",
			"pod_memory_utilization_over_pod_limit",
			"node_memory_limit",
			"pod_cpu_request",
			"pod_interface_network_tx_dropped",
			"pod_status_succeeded",
			"namespace_number_of_running_pods",
			"pod_memory_reserved_capacity",
			"node_diskio_io_serviced_total",
			"pod_network_rx_bytes",
			"node_status_capacity_pods",
			"pod_status_unknown",
			"cluster_failed_node_count",
			"container_memory_utilization",
			"node_memory_utilization",
			"node_filesystem_inodes_free",
			"container_memory_request",
			"container_cpu_limit",
			"node_memory_reserved_capacity",
			"node_interface_network_tx_dropped",
			"pod_cpu_utilization_over_pod_limit",
			"container_memory_failures_total",
			"pod_status_ready",
			"pod_number_of_running_containers",
			"cluster_node_count",
			"pod_memory_request",
			"node_cpu_utilization",
			"cluster_number_of_running_pods",
			"node_memory_working_set",
			"pod_status_failed",
			"node_status_condition_pid_pressure",
			"pod_status_scheduled",
			"node_number_of_running_containers",
			"node_cpu_limit",
			"node_status_condition_disk_pressure",
			"pod_cpu_limit",
			"pod_memory_limit",
			"node_cpu_usage_total",
			"pod_cpu_reserved_capacity",
			"pod_network_tx_bytes",
			"container_memory_limit",
			"pod_memory_utilization",
			"node_interface_network_rx_dropped",
			"node_network_total_bytes",
			"container_cpu_utilization_over_container_limit",
			"pod_interface_network_rx_dropped",
			"pod_container_status_waiting",
			"node_cpu_reserved_capacity",
			"container_memory_utilization_over_container_limit",
			"container_cpu_request",
			"pod_cpu_usage_total",
			"pod_memory_working_set",
			"pod_container_status_waiting_reason_crash_loop_back_off",
		},
		"ClusterName-FullPodName-Namespace-PodName": {
			"pod_network_tx_bytes",
			"pod_interface_network_rx_dropped",
			"pod_cpu_limit",
			"pod_status_succeeded",
			"pod_container_status_waiting",
			"pod_number_of_running_containers",
			"pod_number_of_container_restarts",
			"pod_status_pending",
			"pod_status_running",
			"pod_container_status_running",
			"pod_memory_limit",
			"pod_status_unknown",
			"pod_memory_utilization_over_pod_limit",
			"pod_cpu_request",
			"pod_status_scheduled",
			"pod_memory_utilization",
			"pod_status_failed",
			"pod_network_rx_bytes",
			"pod_number_of_containers",
			"pod_cpu_utilization",
			"pod_memory_reserved_capacity",
			"pod_status_ready",
			"pod_container_status_terminated",
			"pod_interface_network_tx_dropped",
			"pod_memory_request",
			"pod_cpu_reserved_capacity",
			"pod_cpu_utilization_over_pod_limit",
			"pod_cpu_usage_total",
			"pod_memory_working_set",
			"pod_container_status_waiting_reason_crash_loop_back_off",
		},
		"ClusterName-Namespace-PodName": {
			"pod_interface_network_rx_dropped",
			"pod_status_succeeded",
			"pod_container_status_running",
			"pod_network_rx_bytes",
			"pod_cpu_utilization",
			"pod_memory_utilization",
			"pod_interface_network_tx_dropped",
			"pod_status_ready",
			"pod_container_status_terminated",
			"pod_cpu_reserved_capacity",
			"pod_memory_request",
			"pod_status_running",
			"pod_status_pending",
			"pod_number_of_containers",
			"pod_memory_utilization_over_pod_limit",
			"pod_status_unknown",
			"pod_cpu_limit",
			"pod_container_status_waiting",
			"pod_memory_reserved_capacity",
			"pod_network_tx_bytes",
			"pod_status_failed",
			"pod_number_of_running_containers",
			"pod_number_of_container_restarts",
			"pod_cpu_request",
			"pod_cpu_utilization_over_pod_limit",
			"pod_status_scheduled",
			"pod_memory_limit",
			"pod_cpu_usage_total",
			"pod_memory_working_set",
			"pod_container_status_waiting_reason_crash_loop_back_off",
		},

		"ClusterName-InstanceId-NodeName": {
			"node_status_allocatable_pods",
			"node_network_total_bytes",
			"node_status_condition_unknown",
			"node_interface_network_rx_dropped",
			"node_number_of_running_containers",
			"node_interface_network_tx_dropped",
			"node_memory_utilization",
			"node_cpu_limit",
			"node_status_condition_disk_pressure",
			"node_memory_working_set",
			"node_cpu_reserved_capacity",
			"node_status_condition_ready",
			"node_filesystem_utilization",
			"node_status_condition_memory_pressure",
			"node_memory_limit",
			"node_memory_reserved_capacity",
			"node_diskio_io_serviced_total",
			"node_status_condition_pid_pressure",
			"node_filesystem_inodes",
			"node_cpu_usage_total",
			"node_number_of_running_pods",
			"node_diskio_io_service_bytes_total",
			"node_status_capacity_pods",
			"node_filesystem_inodes_free",
			"node_cpu_utilization",
		},

		"ClusterName-Namespace-Service": {
			"pod_status_unknown",
			"pod_memory_limit",
			"pod_container_status_terminated",
			"pod_status_ready",
			"pod_number_of_container_restarts",
			"pod_status_pending",
			"pod_status_succeeded",
			"pod_network_rx_bytes",
			"pod_status_failed",
			"pod_number_of_containers",
			"pod_cpu_request",
			"service_number_of_running_pods",
			"pod_memory_reserved_capacity",
			"pod_network_tx_bytes",
			"pod_container_status_waiting",
			"pod_memory_request",
			"pod_status_running",
			"pod_container_status_running",
			"pod_cpu_reserved_capacity",
			"pod_memory_utilization_over_pod_limit",
			"pod_cpu_utilization",
			"pod_memory_utilization",
			"pod_number_of_running_containers",
			"pod_status_scheduled",
			"pod_cpu_usage_total",
			"pod_memory_working_set",
		},
		"ClusterName-Namespace": {
			"pod_interface_network_rx_dropped",
			"pod_network_rx_bytes",
			"pod_cpu_utilization_over_pod_limit",
			"pod_memory_utilization_over_pod_limit",
			"namespace_number_of_running_pods",
			"pod_memory_utilization",
			"pod_interface_network_tx_dropped",
			"pod_cpu_utilization",
			"pod_network_tx_bytes",
		},
	}

	if env.InstancePlatform == "windows" {
		ExpectedDimsToMetrics["ClusterName"] = append(ExpectedDimsToMetrics["ClusterName"],
			"container_filesystem_usage", "container_filesystem_available", "container_filesystem_utilization")
	}

	return ExpectedDimsToMetrics
}
