[
  {
    "name": "redis-0",
    "image": "redis:6.0.8-alpine3.12",
    "essential": true,
    "portMappings": [
      {
        "protocol": "tcp",
        "containerPort": 6379
      }
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-region": "${region}",
        "awslogs-stream-prefix": "memcached-tutorial",
        "awslogs-group": "${log_group}"
      }
    },
    "cpu": 128,
    "mountPoints": [ ],
    "memory": 512,
    "volumesFrom": [ ]
  },
  {
    "name": "redis-exporter-0",
    "image": "oliver006/redis_exporter:v1.11.1-alpine",
    "essential": true,
    "portMappings": [
      {
        "protocol": "tcp",
        "containerPort": 9121
      }
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-region": "${region}",
        "awslogs-stream-prefix": "memcached-exporter-tutorial",
        "awslogs-group": "${log_group}"
      }
    },
    "cpu": 128,
    "mountPoints": [ ],
    "memory": 512,
    "volumesFrom": [ ]
  }
]
