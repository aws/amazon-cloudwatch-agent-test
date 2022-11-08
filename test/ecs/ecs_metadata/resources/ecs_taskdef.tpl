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
    "memory": 1024,
    "volumesFrom": []
  }
]
