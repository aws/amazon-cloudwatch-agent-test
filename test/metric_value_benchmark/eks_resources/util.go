// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eks_resources

import _ "embed"

var (
	//go:embed test_schemas/cluster.json
	eksClusterSchema string
	//go:embed test_schemas/cluster_namespace.json
	eksClusterNamespaceSchema string
	//go:embed test_schemas/cluster_service.json
	eksClusterServiceSchema string
	//go:embed test_schemas/container.json
	eksContainerSchema string
	//go:embed test_schemas/container_fs.json
	eksContainerFSSchema string
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
		"Cluster":          eksClusterSchema,
		"ClusterNamespace": eksClusterNamespaceSchema,
		"ClusterService":   eksClusterServiceSchema,
		"Container":        eksContainerSchema,
		"ContainerFS":      eksContainerFSSchema,
		"Node":             eksNodeSchema,
		"NodeDiskIO":       eksNodeDiskIOSchema,
		"NodeFS":           eksNodeFSSchema,
		"NodeNet":          eksNodeNetSchema,
		"Pod":              eksPodSchema,
		"PodNet":           eksPodNetSchema,
	}
)
