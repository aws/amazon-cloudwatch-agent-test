#!/bin/bash
# Sends an OTLP log with attributes["event.name"] = "aws.service_events.aggregate_profile"
# to test routing to the no-batch pipeline.
# Usage: ./send_otlp_log_aggregate_profile.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]

SERVICE_NAME="${1:?Usage: send_otlp_log_aggregate_profile.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]}"
NUM_LOGS="${2:?Usage: send_otlp_log_aggregate_profile.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]}"
HOST_NAME="${3:-test-host}"
INSTANCE_ID="${4:-instance-001}"
ENDPOINT="${5:-http://127.0.0.1:4316/v1/logs}"

for i in $(seq 1 "$NUM_LOGS"); do
    TIMESTAMP_NS=$(date +%s%N)
    curl -s -o /dev/null -w "%{http_code}" \
        -X POST "$ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{
  \"resourceLogs\": [{
    \"resource\": {
      \"attributes\": [
        {\"key\": \"service.name\", \"value\": {\"stringValue\": \"$SERVICE_NAME\"}},
        {\"key\": \"host.name\", \"value\": {\"stringValue\": \"$HOST_NAME\"}},
        {\"key\": \"service.instance.id\", \"value\": {\"stringValue\": \"$INSTANCE_ID\"}}
      ]
    },
    \"scopeLogs\": [{
      \"scope\": { \"name\": \"test-logger\" },
      \"logRecords\": [{
        \"timeUnixNano\": \"$TIMESTAMP_NS\",
        \"severityNumber\": 9,
        \"severityText\": \"INFO\",
        \"body\": { \"stringValue\": \"Aggregate profile log $i from $SERVICE_NAME\" },
        \"attributes\": [
          {\"key\": \"event.name\", \"value\": {\"stringValue\": \"aws.service_events.aggregate_profile\"}}
        ]
      }]
    }]
  }]
}"
    echo " - sent aggregate_profile log $i for $SERVICE_NAME"
    sleep 0.1
done
