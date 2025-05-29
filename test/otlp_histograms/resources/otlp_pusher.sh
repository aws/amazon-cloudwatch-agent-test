#!/bin/bash

# Get instance ID from instance metadata
INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)

# Initialize start time and counters
INITIAL_START_TIME=$(date +%s%N)
COUNT=0

# Create initial template with INITIAL_START_TIME and INSTANCE_ID
cat ./resources/otlp_metrics.json | \
    sed -e "s/INITIAL_START_TIME/$INITIAL_START_TIME/" \
    -e "s/\$INSTANCE_ID/$INSTANCE_ID/g" > ./resources/otlp_metrics_initial.json

while true
do
    # Get current timestamp
    START_TIME=$(date +%s%N)

    # Increment counters
    COUNT=$((COUNT + 2))
    SUM=$COUNT  # In this case, SUM equals COUNT, modify if needed

    # Create the final metrics file with all replacements
    cat ./resources/otlp_metrics_initial.json | \
        sed -e "s/\$START_TIME/$START_TIME/g" \
        -e "s/CUMULATIVE_HIST_COUNT/$COUNT/g" \
        -e "s/CUMULATIVE_HIST_SUM/$SUM/g" > otlp_metrics.json

    # Send the metrics
    curl -H 'Content-Type: application/json' \
         -d @otlp_metrics.json \
         -i http://127.0.0.1:1234/v1/metrics --verbose

    sleep 10
done