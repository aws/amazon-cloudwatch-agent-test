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
            "memory": 2048
        },
        {
        "name": "cloudwatch_agent",
        "image": "bionic-20230308",
        "essential": true,
        "entryPoint": [
            "/bin/sh",
            "-c",
            "cat <<EOF | sudo tee /etc/emf.sh
{*            TOKEN=\$(curl -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")*}
{*            INSTANCEID=\$(curl -H "X-aws-ec2-metadata-token: \${TOKEN}" -v http://169.254.169.254/latest/meta-data/instance-id)*}
            INSTANCEID = \$(curl \${ECS_CONTAINER_METADATA_URI} -H "DockerId")
            while true;
            do
            CURRENT_TIME=\$(date +%s%N | cut -b1-13)
            echo '{"_aws":{"Timestamp":'"\${CURRENT_TIME}"',"LogGroupName":"MetricValueBenchmarkTest","CloudWatchMetrics":[{"Namespace":"EMFNameSpace","Dimensions":[["Type","InstanceId"]],"Metrics":[{"Name":"EMFCounter","Unit":"Count"}]}]},"Type":"Counter","EMFCounter":5,"InstanceId":"'"\${INSTANCEID}"'"}' \ > /dev/udp/0.0.0.0/25888
            sleep 60
            done
            EOF; done"
        ],
    }
]
