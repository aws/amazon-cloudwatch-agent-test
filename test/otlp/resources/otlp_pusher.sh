#!/bin/bash

if [ -z "$INSTANCE_ID" ]; then
    echo "Error: INSTANCE_ID environment variable is not set"
    exit 1
fi

echo "Starting OTLP data generation with INSTANCE_ID: $INSTANCE_ID"

COUNTER=0
SEQUENCE=0

while true; do
    TIMESTAMP_NANO=$(date +%s%N)
    START_TIME_NANO=$TIMESTAMP_NANO
    SEQUENCE=$((SEQUENCE + 1))
    COUNTER=$((COUNTER + 1))

    # Generate metrics
    cat ./resources/otlp_metrics.json | \
        sed -e "s/TIMESTAMP_NANO/$TIMESTAMP_NANO/g" \
        -e "s/START_TIME_NANO/$START_TIME_NANO/g" \
        -e "s/COUNTER_VALUE/$COUNTER/g" \
        -e "s/GAUGE_VALUE/$((RANDOM % 100))/g" \
        -e "s/\$INSTANCE_ID/$INSTANCE_ID/g" > otlp_metrics_temp.json

    curl -s -X POST http://127.0.0.1:4318/v1/metrics \
        -H "Content-Type: application/json" \
        -d @otlp_metrics_temp.json

    # Generate logs
    cat ./resources/otlp_logs.json | \
        sed -e "s/TIMESTAMP_NANO/$TIMESTAMP_NANO/g" \
        -e "s/START_TIME_NANO/$START_TIME_NANO/g" \
        -e "s/COUNTER_VALUE/$((COUNTER + 2))/g" \
        -e "s/GAUGE_VALUE/$((RANDOM % 100))/g" \
        -e "s/\$INSTANCE_ID/$INSTANCE_ID/g" > otlp_logs_temp.json

    curl -s -X POST http://127.0.0.1:4318/v1/metrics \
        -H "Content-Type: application/json" \
        -d @otlp_logs_temp.json -v

    sleep 10
done
