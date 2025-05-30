#!/bin/bash

if [ -z "$INSTANCE_ID" ]; then
    echo "INSTANCE_ID environment variable is not set"
    exit 1
fi

# Create the initial JSON payload
cat <<EOF > /tmp/metrics_payload.json
{
  "resourceMetrics": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": {
              "stringValue": "my.service"
            }
          }
        ]
      },
      "scopeMetrics": [
        {
          "scope": {
            "name": "my.library",
            "version": "1.0.0",
            "attributes": [
              {
                "key": "my.scope.attribute",
                "value": {
                  "stringValue": "some scope attribute"
                }
              }
            ]
          },
          "metrics": [
            {
              "name": "my.delta.histogram",
              "unit": "1",
              "description": "I am a Delta Histogram",
              "histogram": {
                "aggregationTemporality": 1,
                "dataPoints": [
                  {
                    "startTimeUnixNano": START_TIME,
                    "timeUnixNano": START_TIME,
                    "count": 2,
                    "sum": 2,
                    "bucketCounts": [0,2],
                    "explicitBounds": [1,2],
                    "min": 0,
                    "max": 2,
                    "attributes": [
                      {
                        "key": "my.delta.histogram.attr",
                        "value": {
                          "stringValue": "some value"
                        }
                      },
                      {
                        "key": "instance_id",
                        "value": {
                          "stringValue": "$INSTANCE_ID"
                        }
                      }
                    ]
                  }
                ]
              }
            },
            {
              "name": "my.cumulative.exponential.histogram",
              "unit": "1",
              "description": "I am an Cumulative Exponential Histogram",
              "exponentialHistogram": {
                "aggregationTemporality": 2,
                "dataPoints": [
                  {
                    "startTimeUnixNano": START_TIME,
                    "timeUnixNano": START_TIME,
                    "count": 3,
                    "sum": 10,
                    "scale": 0,
                    "zeroCount": 1,
                    "positive": {
                      "offset": 1,
                      "bucketCounts": [0,2]
                    },
                    "min": 0,
                    "max": 5,
                    "zeroThreshold": 0,
                    "attributes": [
                      {
                        "key": "my.cumulative.exponential.histogram.attr",
                        "value": {
                          "stringValue": "some value"
                        }
                      },
                      {
                        "key": "instance_id",
                        "value": {
                          "stringValue": "$INSTANCE_ID"
                        }
                      }
                    ]
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}
EOF

# Start the OTLP sending loop
while true; do
    START_TIME=$(date +%s%N)

    cat /tmp/metrics_payload.json | \
        sed -e "s/START_TIME/$START_TIME/g" > /tmp/metrics_payload_with_time.json

    response=$(curl -s -w "\n%{http_code}" -X POST http://127.0.0.1:1234/v1/metrics \
      -H "Content-Type: application/json" \
      -d @/tmp/metrics_payload_with_time.json)

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" -ne 200 ]; then
        echo "Failed to send metrics. Status code: $http_code"
        echo "Response body: $body"
        echo "Retrying in 10 seconds..."
    else
        echo "Successfully sent metrics at $(date)"
    fi

    rm -f /tmp/metrics_payload_with_time.json

    sleep 10
done

rm -f /tmp/metrics_payload.json