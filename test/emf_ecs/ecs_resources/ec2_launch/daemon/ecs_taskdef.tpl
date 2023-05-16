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
        "image": "ubuntu:20.04",
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
            "INSTANCEID=\"$(curl $${ECS_CONTAINER_METADATA_URI_V4} | sed -n 's|.*\"ContainerARN\": *\"\\([^\"]*\\)\".*|\\1|p')instance\"; CLUSTER_NAME=\"$(curl $${ECS_CONTAINER_METADATA_URI_V4}/task | sed -n 's|.*\"Cluster\": *\"\\([^\"]*\\)\".*|\\1|p')cluster\"; CONTAINER_ID=\"$(curl $${ECS_CONTAINER_METADATA_URI_V4} | sed -n 's|.*\"DockerId\": *\"\\([^\"]*\\)\".*|\\1|p')containerID\"; while true; do CURRENT_TIME=\"$(date +%s%3N)\"; echo '{\"_aws\":{\"Timestamp\":'\"$${CURRENT_TIME}\"',\"LogGroupName\":\"EMFECSLogGroup\",\"CloudWatchMetrics\":[{\"Namespace\":\"EMFNameSpace\",\"Dimensions\":[[\"Type\",\"InstanceId\"]],\"Metrics\":[{\"Name\":\"EMFCounter\",\"Unit\":\"Count\"}]}]},\"Type\":\"Counter\",\"EMFCounter\":5}' | socat -v -t 0 - UDP:cloudwatch_agent:25888; sleep 60; done"
        ]
    }
]