#!/bin/sh
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT
#
# OTLP sender for the Kubernetes tests of the chart's OTLP endpoint. It runs in an ordinary pod (no
# hostNetwork) and posts OTLP/HTTP JSON to the cloudwatch-agent Service, the way an application does.
# It sets no pod or node attributes, so the agent has to find the pod from the connection's source IP.
#
# Unlike otlp_load_generator.sh, this file is not a Terraform template: it is mounted from a ConfigMap
# as is, so shell expansions are written normally.
#
# Environment:
#   OTLP_ENDPOINT      base URL of the receiver (default http://cloudwatch-agent.amazon-cloudwatch:4318)
#   OTLP_CASE          value of the otlp_e2e_case attribute on every metric data point, log record and span
#   OTLP_SERVICE_NAME  service.name resource attribute to send; empty sends none
#   OTLP_METRICS       space-separated names of the counters to send (default otlp_e2e_requests)
#   OTLP_ITERATIONS    rounds to send; 0 sends until the pod stops (default 0)
#   OTLP_INTERVAL      seconds between rounds (default 10)
#
# Every post prints "otlp-e2e-send <signal> <HTTP status>" to stdout, with status 000 when the
# connection fails, so a test can read from the pod log whether the endpoint answered.

ENDPOINT="${OTLP_ENDPOINT:-http://cloudwatch-agent.amazon-cloudwatch:4318}"
CASE="${OTLP_CASE:-default}"
METRICS="${OTLP_METRICS:-otlp_e2e_requests}"
ITERATIONS="${OTLP_ITERATIONS:-0}"
INTERVAL="${OTLP_INTERVAL:-10}"

if [ -n "$OTLP_SERVICE_NAME" ]; then
  RESOURCE="[{\"key\":\"service.name\",\"value\":{\"stringValue\":\"$OTLP_SERVICE_NAME\"}}]"
else
  RESOURCE="[]"
fi
CASE_ATTR="{\"key\":\"otlp_e2e_case\",\"value\":{\"stringValue\":\"$CASE\"}}"
START_NS="$(date +%s)000000000"

hex() {
  od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
}

post() {
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -X POST "$ENDPOINT/v1/$1" \
    -H 'Content-Type: application/json' -d "$2")
  echo "otlp-e2e-send $1 $code"
}

SEQ=0
while [ "$ITERATIONS" -eq 0 ] || [ "$SEQ" -lt "$ITERATIONS" ]; do
  SEQ=$((SEQ + 1))
  NOW_S=$(date +%s)
  NOW_NS="${NOW_S}000000000"

  POINTS=""
  for name in $METRICS; do
    [ -n "$POINTS" ] && POINTS="$POINTS,"
    POINTS="$POINTS{\"name\":\"$name\",\"unit\":\"1\",\"sum\":{\"aggregationTemporality\":2,\"isMonotonic\":true,\"dataPoints\":[{\"asInt\":\"$SEQ\",\"startTimeUnixNano\":\"$START_NS\",\"timeUnixNano\":\"$NOW_NS\",\"attributes\":[$CASE_ATTR]}]}}"
  done
  post metrics "{\"resourceMetrics\":[{\"resource\":{\"attributes\":$RESOURCE},\"scopeMetrics\":[{\"scope\":{\"name\":\"otlp-e2e\"},\"metrics\":[$POINTS]}]}]}"

  post logs "{\"resourceLogs\":[{\"resource\":{\"attributes\":$RESOURCE},\"scopeLogs\":[{\"scope\":{\"name\":\"otlp-e2e\"},\"logRecords\":[{\"timeUnixNano\":\"$NOW_NS\",\"severityText\":\"INFO\",\"body\":{\"stringValue\":\"otlp-e2e $CASE\"},\"attributes\":[$CASE_ATTR]}]}]}]}"

  # X-Ray needs the first 4 bytes of the trace ID to be the Unix time in seconds. The span is a
  # server span with HTTP attributes, so the span metrics connector has dimensions to report.
  TRACE_ID="$(printf '%08x' "$NOW_S")$(hex 12)"
  post traces "{\"resourceSpans\":[{\"resource\":{\"attributes\":$RESOURCE},\"scopeSpans\":[{\"scope\":{\"name\":\"otlp-e2e\"},\"spans\":[{\"traceId\":\"$TRACE_ID\",\"spanId\":\"$(hex 8)\",\"name\":\"GET /e2e\",\"kind\":2,\"startTimeUnixNano\":\"$((NOW_S - 1))000000000\",\"endTimeUnixNano\":\"$NOW_NS\",\"attributes\":[$CASE_ATTR,{\"key\":\"http.request.method\",\"value\":{\"stringValue\":\"GET\"}},{\"key\":\"http.route\",\"value\":{\"stringValue\":\"/e2e\"}},{\"key\":\"http.response.status_code\",\"value\":{\"intValue\":\"200\"}}]}]}]}]}"

  sleep "$INTERVAL"
done
