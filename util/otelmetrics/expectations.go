// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

// Shared resource attribute expectations for OTEL Container Insights telemetry.
// These define the canonical sets of resource attributes expected at each scope
// level. Both metrics (PromQL) and logs (Logs Insights) tests should reference
// these to ensure cross-telemetry consistency.

// NodeResourceAttrs are resource attributes expected on all node-scoped
// telemetry: node_exporter metrics, host logs, kubeletstats node metrics.
// These come from resourcedetection/ec2 + k8sattributes/node + cluster name.
var NodeResourceAttrs = []string{
	"k8s.cluster.name",
	"k8s.node.name",
	"k8s.node.uid",
	"host.id",
	"host.type",
	"host.name",
	"host.image.id",
	"cloud.provider",
	"cloud.platform",
	"cloud.region",
	"cloud.availability_zone",
	"cloud.account.id",
	"cloud.resource_id",
}

// PodResourceAttrs are resource attributes expected on pod-scoped telemetry:
// cadvisor metrics, kubeletstats pod metrics, application logs.
// Superset of NodeResourceAttrs plus pod identity.
var PodResourceAttrs = append(append([]string(nil), NodeResourceAttrs...),
	"k8s.pod.name",
	"k8s.namespace.name",
	"k8s.pod.uid",
)

// ContainerResourceAttrs are resource attributes expected on container-scoped
// telemetry: cadvisor container metrics, application logs.
// Superset of PodResourceAttrs plus container identity.
var ContainerResourceAttrs = append(append([]string(nil), PodResourceAttrs...),
	"k8s.container.name",
)

// WorkloadAttrs are resource attributes set by workload derivation.
// Present on pod/container-scoped telemetry when the pod belongs to a
// known workload (Deployment, StatefulSet, DaemonSet, Job, CronJob, ReplicaSet).
var WorkloadAttrs = []string{
	"k8s.workload.name",
	"k8s.workload.type",
}

// AppLogResourceAttrs are resource attributes expected on application logs.
// Same as ContainerResourceAttrs + workload + service.name.
var AppLogResourceAttrs = append(append([]string(nil), ContainerResourceAttrs...),
	"k8s.workload.name",
	"k8s.workload.type",
	"service.name",
)

// HostLogResourceAttrs are resource attributes expected on host logs.
// Node-level only — no pod, container, or workload context.
var HostLogResourceAttrs = []string{
	"k8s.cluster.name",
	"k8s.node.name",
	"host.id",
	"host.type",
	"host.name",
	"host.image.id",
	"cloud.provider",
	"cloud.platform",
	"cloud.region",
	"cloud.availability_zone",
	"cloud.account.id",
	"cloud.resource_id",
}

// HostLogAbsentAttrs are resource attributes that must NOT be present on host logs.
var HostLogAbsentAttrs = []string{
	"k8s.pod.name",
	"k8s.pod.uid",
	"k8s.namespace.name",
	"k8s.container.name",
	"k8s.workload.name",
	"k8s.workload.type",
	"service.name",
}

// ScopeAttrs are instrumentation scope attributes expected on all Container
// Insights telemetry (both metrics and logs).
var ScopeAttrs = map[string]string{
	"cloudwatch.source":   "cloudwatch-agent",
	"cloudwatch.solution": "k8s-otel-container-insights",
}

// AppLogScopeAttrs are scope attributes specific to the application logs pipeline.
var AppLogScopeAttrs = map[string]string{
	"cloudwatch.source":   "cloudwatch-agent",
	"cloudwatch.solution": "k8s-otel-container-insights",
	"cloudwatch.pipeline": "application-logs",
}

// HostLogScopeAttrs are scope attributes specific to the host logs pipeline.
var HostLogScopeAttrs = map[string]string{
	"cloudwatch.source":   "cloudwatch-agent",
	"cloudwatch.solution": "k8s-otel-container-insights",
	"cloudwatch.pipeline": "host-logs",
}
