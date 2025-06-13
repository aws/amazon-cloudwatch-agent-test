#!/bin/bash

if [ -z "$INSTANCE_ID" ]; then
    echo "Error: INSTANCE_ID environment variable is not set"
    exit 1
fi

echo "Starting with INSTANCE_ID: $INSTANCE_ID"

INITIAL_START_TIME=$(date +%s%N)
COUNT=0

# Initialize the file
cat ./resources/otlp_emf_metrics.json | sed -e "s/INITIAL_START_TIME/$INITIAL_START_TIME/" \
    -e "s/\$INSTANCE_ID/$INSTANCE_ID/g" > ./resources/otlp_emf_metrics_initial.json

while true; do
    START_TIME=$(date +%s%N)
    COUNT=$((COUNT + 2))
    SUM=$((COUNT))

    cat ./resources/otlp_emf_metrics_initial.json | \
        sed -e "s/START_TIME/$START_TIME/" \
        -e "s/CUMULATIVE_HIST_COUNT/$COUNT/" \
        -e "s/CUMULATIVE_HIST_SUM/$SUM/" > otlp_emf_metrics.json

    echo "Sending metrics with INSTANCE_ID: $INSTANCE_ID"
    curl -H 'Content-Type: application/json' -d @otlp_emf_metrics.json -i http://127.0.0.1:1234/v1/metrics --verbose
    sleep 10
done