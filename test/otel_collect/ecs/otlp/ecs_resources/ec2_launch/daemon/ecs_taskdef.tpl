[
  {
    "name": "cloudwatch_agent",
    "image": "${cwagent_image}",
    "essential": true,
    "secrets": [
      {
        "name": "CW_CONFIG_CONTENT",
        "valueFrom": "${cwagent_ssm_parameter_arn}"
      },
      {
        "name": "PROMETHEUS_CONFIG_CONTENT",
        "valueFrom": "${prometheus_ssm_parameter_arn}"
      }
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-region": "${region}",
        "awslogs-stream-prefix": "${testing_id}",
        "awslogs-group": "${log_group}"
      }
    },
    "cpu": 1,
    "memory": 1792
  },
  {
    "name": "otlp_pusher",
    "image": "curlimages/curl:8.10.1",
    "essential": false,
    "links": [
      "cloudwatch_agent"
    ],
    "environment": [
      {
        "name": "TEST_ID",
        "value": "${testing_id}"
      }
    ],
    "entryPoint": [
      "sh",
      "-c"
    ],
    "command": [
      "while true; do S=$$(date +%s); NOW=$${S}000000000; START=$$(expr $${S} - 10)000000000; printf '{\"resourceMetrics\":[{\"resource\":{\"attributes\":[{\"key\":\"TestId\",\"value\":{\"stringValue\":\"%s\"}}]},\"scopeMetrics\":[{\"metrics\":[{\"name\":\"otlp_test_counter\",\"sum\":{\"dataPoints\":[{\"asInt\":\"1\",\"startTimeUnixNano\":\"%s\",\"timeUnixNano\":\"%s\",\"attributes\":[{\"key\":\"TestId\",\"value\":{\"stringValue\":\"%s\"}}]}],\"aggregationTemporality\":1,\"isMonotonic\":true}},{\"name\":\"otlp_test_gauge\",\"gauge\":{\"dataPoints\":[{\"asDouble\":42.0,\"timeUnixNano\":\"%s\",\"attributes\":[{\"key\":\"TestId\",\"value\":{\"stringValue\":\"%s\"}}]}]}}]}]}]}' \"$${TEST_ID}\" \"$${START}\" \"$${NOW}\" \"$${TEST_ID}\" \"$${NOW}\" \"$${TEST_ID}\" > /tmp/p.json; curl -s -X POST http://cloudwatch_agent:4318/v1/metrics -H 'Content-Type: application/json' -d @/tmp/p.json; sleep 10; done"
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-region": "${region}",
        "awslogs-stream-prefix": "${testing_id}-pusher",
        "awslogs-group": "${log_group}"
      }
    },
    "cpu": 1,
    "memory": 256
  }
]
