[
  {
    "name": "cloudwatch_agent",
    "image": "${cwagent_image}",
    "essential": true,
    "secrets": [
      {
        "name": "CW_CONFIG_CONTENT",
        "valueFrom": "${cwagent_ssm_parameter_arn}"
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
    "cpu": 128,
    "mountPoints": [
        {
            "readOnly": true,
            "containerPath": "/rootfs/proc",
            "sourceVolume": "proc"
        },
        {
          "readOnly": true,
          "containerPath": "/rootfs/dev",
          "sourceVolume": "dev"
        },
        {
          "readOnly": true,
          "containerPath": "/sys/fs/cgroup",
          "sourceVolume": "al2_cgroup"
        },
        {
          "readOnly": true,
          "containerPath": "/cgroup",
          "sourceVolume": "al1_cgroup"
        },
        {
          "readOnly": true,
          "containerPath": "/rootfs/sys/fs/cgroup",
          "sourceVolume": "al2_cgroup"
        },
        {
          "readOnly": true,
          "containerPath": "/rootfs/cgroup",
          "sourceVolume": "al1_cgroup"
        }
    ],
    "memory": 512,
    "portMappings": [
      {
          "containerPort": 8125,
          "protocol": "udp"
      }
    ]
  },
  {
      "name": "statsd-client",
      "image": "alpine/socat:latest",
      "essential": true,
      "entryPoint": [
          "/bin/sh",
          "-c",
          "while true; do echo 'statsd_counter_1:1.0|c|#key:value' | socat -v -t 0 - UDP:cloudwatch_agent:8125; echo 'statsd_gauge_1:1.0|g|#key:value' | socat -v -t 0 - UDP:cloudwatch_agent:8125; sleep 1; done"
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-region": "${region}",
          "awslogs-stream-prefix": "statsd-test",
          "awslogs-group": "${log_group}"
        }
      },
      "cpu": 128,
      "mountPoints": [ ],
      "memory": 64,
      "volumesFrom": [ ],
      "links": [
          "cloudwatch_agent"
      ]
  }
]
