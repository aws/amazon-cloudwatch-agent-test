#!/bin/bash

CURRENT_TIME=$(date +%s%N)
START_TIME=$CURRENT_TIME

if [ -z "$INSTANCE_ID" ]; then
    echo "INSTANCE_ID environment variable is not set"
    exit 1
fi

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
          },
          {
            "key": "custom.attribute",
            "value": {
              "stringValue": "test-value"
            }
          },
          {
            "key": "environment",
            "value": {
              "stringValue": "production"
            }
          },
          {
            "key": "instance_id",
            "value": {
              "stringValue": "$INSTANCE_ID"
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
                    "startTimeUnixNano": $START_TIME,
                    "timeUnixNano": $START_TIME,
                    "count": 2,
                    "sum": 2,
                    "bucketCounts": [1,1],
                    "explicitBounds": [1],
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
                        "key": "region",
                        "value": {
                          "stringValue": "us-west-2"
                        }
                      },
                      {
                        "key": "status",
                        "value": {
                          "stringValue": "active"
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

response=$(curl -s -w "\n%{http_code}" -X POST http://127.0.0.1:1234/v1/metrics \
  -H "Content-Type: application/json" \
  -d @/tmp/metrics_payload.json)

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" -ne 200 ]; then
    echo "Failed to send metrics. Status code: $http_code"
    echo "Response body: $body"
    exit 1
fi

rm -f /tmp/metrics_payload.json

exit 0