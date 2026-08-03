#!/bin/bash
# Sends OTLP logs WITHOUT a service.name resource attribute to test
# DefaultPlaceholderValue fallback behavior.
# Usage: ./send_otlp_logs_no_service.sh <num_logs> [endpoint]

NUM_LOGS="${1:?Usage: send_otlp_logs_no_service.sh <num_logs> [endpoint]}"
ENDPOINT="${2:-http://127.0.0.1:4316/v1/logs}"

for i in $(seq 1 "$NUM_LOGS"); do
    TIMESTAMP_NS=$(date +%s%N)
    curl -s -o /dev/null -w "%{http_code}" \
        -X POST "$ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{
  \"resourceLogs\": [{
    \"resource\": {
      \"attributes\": []
    },
    \"scopeLogs\": [{
      \"scope\": { \"name\": \"test-logger\" },
      \"logRecords\": [{
        \"timeUnixNano\": \"$TIMESTAMP_NS\",
        \"severityNumber\": 9,
        \"severityText\": \"INFO\",
        \"body\": { \"stringValue\": \"Test log $i with no service name\" }
      }]
    }]
  }]
}"
    echo " - sent log $i (no service.name)"
    sleep 0.1
done
