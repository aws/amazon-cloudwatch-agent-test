#!/bin/bash
# Sends OTLP metrics. If TELEMETRY_SOURCE is set to "ServiceEvents", adds that as a
# datapoint attribute to trigger routing to the OTLP monitoring endpoint.
# Usage: ./send_otlp_metrics.sh <metric_file_or_name> <num_datapoints> [telemetry_source] [endpoint]
#
# If metric_file_or_name is a .json file path, sends that file directly.
# Otherwise, sends a simple gauge metric with the given name.

METRIC_INPUT="${1:?Usage: send_otlp_metrics.sh <metric_file_or_name> <num_datapoints> [telemetry_source] [endpoint]}"
NUM="${2:?Usage: send_otlp_metrics.sh <metric_file_or_name> <num_datapoints> [telemetry_source] [endpoint]}"
TELEMETRY_SOURCE="${3:-}"
ENDPOINT="${4:-http://127.0.0.1:4316/v1/metrics}"

# If input is a JSON file, send it (replacing START_TIME placeholder with current timestamp)
if [ -f "$METRIC_INPUT" ]; then
    for i in $(seq 1 "$NUM"); do
        TIMESTAMP_NS=$(date +%s%N)
        sed "s/START_TIME/$TIMESTAMP_NS/g" "$METRIC_INPUT" | \
            curl -s -o /dev/null -w "%{http_code}" \
                -X POST "$ENDPOINT" \
                -H "Content-Type: application/json" \
                -d @-
        echo " - sent metric file $METRIC_INPUT (iteration $i)"
        sleep 0.5
    done
    exit 0
fi

# Otherwise, send a simple gauge metric
METRIC_NAME="$METRIC_INPUT"
DP_ATTRS='[]'
if [ -n "$TELEMETRY_SOURCE" ]; then
    DP_ATTRS='[{"key":"Telemetry.Source","value":{"stringValue":"'"$TELEMETRY_SOURCE"'"}}]'
fi

for i in $(seq 1 "$NUM"); do
    TIMESTAMP_NS=$(date +%s%N)
    curl -s -o /dev/null -w "%{http_code}" \
        -X POST "$ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{
  \"resourceMetrics\": [{
    \"resource\": {
      \"attributes\": [{\"key\":\"service.name\",\"value\":{\"stringValue\":\"metrics-test-svc\"}}]
    },
    \"scopeMetrics\": [{
      \"scope\": {\"name\": \"test-meter\"},
      \"metrics\": [{
        \"name\": \"$METRIC_NAME\",
        \"gauge\": {
          \"dataPoints\": [{
            \"timeUnixNano\": \"$TIMESTAMP_NS\",
            \"asDouble\": $((RANDOM % 100)),
            \"attributes\": $DP_ATTRS
          }]
        }
      }]
    }]
  }]
}"
    echo " - sent metric $METRIC_NAME datapoint $i"
    sleep 0.5
done
