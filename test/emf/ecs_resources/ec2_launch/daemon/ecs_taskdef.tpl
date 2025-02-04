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
        "memory": 2048,
        "portMappings": [
            {
                "containerPort": 25888,
                "protocol": "udp"
            }
        ]
    },
    {
        "name": "emf_container",
        "links":  ["cloudwatch_agent"],
        "image": "busybox:latest",
        "logConfiguration": {
            "logDriver": "awslogs",
            "options": {
                "awslogs-region": "${region}",
                "awslogs-stream-prefix": "emf-test",
                "awslogs-group": "${log_group}"
            }
        },
        "essential": true,
        "entryPoint": [
            "/bin/sh",
            "-c",
            "while true; do CURRENT_TIME=\"$(date +%s%3N)\";  CLUSTER_NAME=\"$(wget -vO- $${ECS_CONTAINER_METADATA_URI_V4}/task | sed -n 's|.*\\\"Cluster\\\": *\\\"\\([^\\\"]*\\)\\\".*|\\1|p')\"; echo '{\"_aws\":{\"Timestamp\":'\"$${CURRENT_TIME}\"',\"LogGroupName\":\"EMFECSLogGroup\",\"CloudWatchMetrics\":[{\"Namespace\":\"EMFECSNameSpace\",\"Dimensions\":[[\"Type\",\"ClusterName\"]],\"Metrics\":[{\"Name\":\"EMFCounter\",\"Unit\":\"Count\"}]}]},\"Type\":\"Counter\",\"EMFCounter\":5, \"ClusterName\": \"'$${CLUSTER_NAME}'\"}' | nc -u -v -w 0 cloudwatch_agent 25888; sleep 60; done"
        ]
    }
]