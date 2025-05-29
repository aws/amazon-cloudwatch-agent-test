#!/bin/bash

# Get instance ID (or set a default for local testing)
INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id || echo "test-instance-1")

# Initial setup
INITIAL_START_TIME=$(date +%s%N)
COUNT=0

# First replacement for INITIAL_START_TIME and INSTANCE_ID
cat ./resources/otlp_metrics.json | \
    sed -e "s/INITIAL_START_TIME/$INITIAL_START_TIME/" \
    -e "s/\$INSTANCE_ID/$INSTANCE_ID/g" > ./resources/otlp_metrics_initial.json

while true
do
    # Get current timestamp
    START_TIME=$(date +%s%N)

    # Update counters
    COUNT=$((COUNT + 2))
    SUM=$COUNT

    # Perform all replacements
    cat ./resources/otlp_metrics_initial.json | \
        sed -e "s/\$START_TIME/$START_TIME/g" \
        -e "s/START_TIME/$START_TIME/g" \
        -e "s/CUMULATIVE_HIST_COUNT/$COUNT/g" \
        -e "s/CUMULATIVE_HIST_SUM/$SUM/g" > otlp_metrics.json

    # Debug output (uncomment if needed)
    # echo "Generated metrics file:"
    # cat otlp_metrics.json

    # Send metrics
    curl -H 'Content-Type: application/json' \
         -d @otlp_metrics.json \
         -i http://127.0.0.1:1234/v1/metrics --verbose

    # Optional: Check if curl was successful
    if [ $? -ne 0 ]; then
        echo "Failed to send metrics"
        sleep 1
    fi

    sleep 10
done