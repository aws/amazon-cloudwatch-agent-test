#!/bin/bash
# Sends OTLP logs with service.name, host.name, and service.instance.id
# resource attributes to test dynamic log group and stream routing.
# Usage: ./send_otlp_logs.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]
#
# Example:
#   ./send_otlp_logs.sh pet-clinic-frontend 5
#   ./send_otlp_logs.sh payment-service 5 my-host inst-001

SERVICE_NAME="${1:?Usage: send_otlp_logs.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]}"
NUM_LOGS="${2:?Usage: send_otlp_logs.sh <service_name> <num_logs> [host_name] [instance_id] [endpoint]}"
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
        \"body\": { \"stringValue\": \"Test log $i from $SERVICE_NAME\" }
      }]
    }]
  }]
}"
    echo " - sent log $i for $SERVICE_NAME"
    sleep 0.1
done
