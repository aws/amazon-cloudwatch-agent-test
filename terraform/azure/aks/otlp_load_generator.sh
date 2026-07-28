#!/bin/sh
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT
#
# OTLP load generator for the AKS integration test. Runs as a Kubernetes Job on the node's network
# namespace so 127.0.0.1:4318 reaches the agent's OTLP receiver on the same node.
#
# Rendered by terraform via templatefile(), so a single-dollar brace expansion is a template
# variable and a double-dollar brace expansion is passed through as a literal shell expansion.
# Every payload carries k8s.cluster.name and k8s.namespace.name so the agent's k8s logs-routing
# template produces a deterministic per-cluster destination, and resourcedetection (which only
# overrides keys it detects, e.g. host.id) leaves them intact.

SERVICE_NAME="${service_name}"
INSTANCE_ID="${instance_id}"
ENDPOINT="${endpoint}"
SEQ=0
START=$(date +%s)
END=$((START + ${duration_seconds}))

while [ $(date +%s) -lt $END ]; do
  SEQ=$((SEQ + 1))
  NOW_S=$(date +%s)
  NOW_NS="$${NOW_S}000000000"
  START_NS="$((NOW_S - 1))000000000"
  # X-Ray requires the first 4 bytes of the trace ID to be the Unix epoch in seconds.
  TRACE_ID=$(printf '%08x0000000000000000%08x' "$NOW_S" "$SEQ")
  SPAN_ID=$(printf '%016x' "$NOW_S$SEQ")

  curl -sf -X POST "$ENDPOINT/v1/metrics" -H "Content-Type: application/json" \
    -d "{\"resourceMetrics\":[{\"resource\":{\"attributes\":[{\"key\":\"service.name\",\"value\":{\"stringValue\":\"$SERVICE_NAME\"}},{\"key\":\"host.id\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.cluster.name\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.namespace.name\",\"value\":{\"stringValue\":\"amazon-cloudwatch\"}}]},\"scopeMetrics\":[{\"scope\":{\"name\":\"aks-otlp-test\"},\"metrics\":[{\"name\":\"aks_otlp_counter\",\"unit\":\"1\",\"sum\":{\"aggregationTemporality\":2,\"isMonotonic\":true,\"dataPoints\":[{\"asInt\":\"$SEQ\",\"startTimeUnixNano\":\"$${START}000000000\",\"timeUnixNano\":\"$NOW_NS\",\"attributes\":[{\"key\":\"ClusterName\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}}]}]}}]}]}]}" || true

  curl -sf -X POST "$ENDPOINT/v1/logs" -H "Content-Type: application/json" \
    -d "{\"resourceLogs\":[{\"resource\":{\"attributes\":[{\"key\":\"service.name\",\"value\":{\"stringValue\":\"$SERVICE_NAME\"}},{\"key\":\"host.id\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.cluster.name\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.namespace.name\",\"value\":{\"stringValue\":\"amazon-cloudwatch\"}}]},\"scopeLogs\":[{\"scope\":{\"name\":\"aks-otlp-test\"},\"logRecords\":[{\"timeUnixNano\":\"$NOW_NS\",\"severityText\":\"INFO\",\"body\":{\"stringValue\":\"aks_otlp_log_$INSTANCE_ID\"},\"attributes\":[{\"key\":\"ClusterName\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}}]}]}]}]}" || true

  curl -sf -X POST "$ENDPOINT/v1/traces" -H "Content-Type: application/json" \
    -d "{\"resourceSpans\":[{\"resource\":{\"attributes\":[{\"key\":\"service.name\",\"value\":{\"stringValue\":\"$SERVICE_NAME\"}},{\"key\":\"host.id\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.cluster.name\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}},{\"key\":\"k8s.namespace.name\",\"value\":{\"stringValue\":\"amazon-cloudwatch\"}}]},\"scopeSpans\":[{\"scope\":{\"name\":\"aks-otlp-test\"},\"spans\":[{\"traceId\":\"$TRACE_ID\",\"spanId\":\"$SPAN_ID\",\"name\":\"aks-otlp-test-span\",\"kind\":2,\"startTimeUnixNano\":\"$START_NS\",\"endTimeUnixNano\":\"$NOW_NS\",\"attributes\":[{\"key\":\"cluster_name\",\"value\":{\"stringValue\":\"$INSTANCE_ID\"}}]}]}]}]}" || true

  sleep 10
done

echo "Load generation complete: $SEQ iterations"
