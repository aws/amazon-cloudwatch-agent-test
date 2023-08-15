module "common" {
  source             = "../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
}

resource "aws_ecs_cluster" "cluster" {
  name = "${local.basename}-${module.common.testing_id}"
}

######
# EC2 for ECS EC2 launch type
######
data "aws_ami" "latest" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn2-ami-ecs-hvm-*-x86_64-ebs"]
  }
}

resource "aws_launch_configuration" "cluster" {
  name          = "cluster-${aws_ecs_cluster.cluster.name}"
  image_id      = data.aws_ami.latest.image_id
  instance_type = var.ec2_instance_type

  security_groups      = [module.basic_components.security_group]
  iam_instance_profile = module.basic_components.instance_profile

  user_data = "#!/bin/bash\necho ECS_CLUSTER=${aws_ecs_cluster.cluster.name} >> /etc/ecs/ecs.config"
  metadata_options {
    http_endpoint               =  "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }
}

resource "aws_autoscaling_group" "cluster" {
  name                 = aws_ecs_cluster.cluster.name
  launch_configuration = aws_launch_configuration.cluster.name
  vpc_zone_identifier  = module.basic_components.public_subnet_ids

  min_size         = 1
  max_size         = 1
  desired_capacity = 1

  tag {
    key                 = "ClusterName"
    value               = aws_ecs_cluster.cluster.name
    propagate_at_launch = true
  }

  tag {
    key                 = "AmazonECSManaged"
    value               = ""
    propagate_at_launch = true
  }

  tag {
    key                 = "BaseClusterName"
    value               = local.basename
    propagate_at_launch = true
  }
}

resource "aws_ecs_capacity_provider" "cluster" {
  name = aws_ecs_cluster.cluster.name

  auto_scaling_group_provider {
    auto_scaling_group_arn = aws_autoscaling_group.cluster.arn

    managed_scaling {
      status                    = "ENABLED"
      maximum_scaling_step_size = 1
      minimum_scaling_step_size = 1
      target_capacity           = 1
    }
  }
}

resource "aws_ecs_cluster_capacity_providers" "cluster" {
  cluster_name = aws_ecs_cluster.cluster.name

  capacity_providers = [aws_ecs_capacity_provider.cluster.name]

  default_capacity_provider_strategy {
    base              = 1
    weight            = 100
    capacity_provider = aws_ecs_capacity_provider.cluster.name
  }
}



resource "aws_cloudwatch_log_group" "log_group" {
  name = "cwagent-integ-test-log-group-${module.common.testing_id}"
}

##########################################
# Template Files
##########################################

locals {
  cwagent_config                = fileexists("../../../${var.test_dir}/resources/config.json") ? "../../../${var.test_dir}/resources/config.json" : "./default_resources/default_amazon_cloudwatch_agent.json"
  cwagent_ecs_taskdef           = fileexists("../../../${var.test_dir}/ecs_resources/ec2_launch/daemon/ecs_taskdef.tpl") ? "../../../${var.test_dir}/ecs_resources/ec2_launch/daemon/ecs_taskdef.tpl" : "./default_resources/default_ecs_taskdef.tpl"
  prometheus_config             = fileexists("../../../${var.test_dir}/resources/ecs_prometheus.tpl") ? "../../../${var.test_dir}/resources/ecs_prometheus.tpl" : "./default_resources/default_ecs_prometheus.tpl"
  extra_apps_ecs_taskdef        = fileexists("../../../${var.test_dir}/resources/extra_apps.tpl") ? "../../../${var.test_dir}/resources/extra_apps.tpl" : "./default_resources/default_extra_apps.tpl"
  cwagent_config_ssm_param_name = "cwagent-integ-test-ssm-config-${module.common.testing_id}"
  basename                      = "cwagent-integ-test-cluster"
}

data "template_file" "cwagent_config" {
  template = file(local.cwagent_config)
  vars = {
  }
}

resource "aws_ssm_parameter" "cwagent_config" {
  name  = local.cwagent_config_ssm_param_name
  type  = "String"
  value = data.template_file.cwagent_config.rendered
}

data "template_file" "prometheus_config" {
  template = file(local.prometheus_config)
  vars = {
  }
}

resource "aws_ssm_parameter" "prometheus_config" {
  name  = "prometheus-integ-test-ssm-config-${module.common.testing_id}"
  type  = "String"
  value = data.template_file.prometheus_config.rendered
}

##########################################
# CloudWatch Agent
##########################################

data "template_file" "cwagent_container_definitions" {
  template = file(local.cwagent_ecs_taskdef)
  vars = {
    region                       = var.region
    cwagent_ssm_parameter_arn    = aws_ssm_parameter.cwagent_config.name
    prometheus_ssm_parameter_arn = aws_ssm_parameter.prometheus_config.name
    cwagent_image                = "${module.common.cwagent_image_repo}:${module.common.cwagent_image_tag}"
    log_group                    = aws_cloudwatch_log_group.log_group.name
    testing_id                   = module.common.testing_id
  }
}

resource "aws_ecs_task_definition" "cwagent_task_definition" {
  family                   = "cwagent-task-family-${module.common.testing_id}"
  network_mode             = "bridge"
  task_role_arn            = module.basic_components.role_arn
  execution_role_arn       = module.basic_components.role_arn
  cpu                      = 256
  memory                   = 2048
  requires_compatibilities = ["EC2"]
  container_definitions    = data.template_file.cwagent_container_definitions.rendered
  depends_on               = [aws_cloudwatch_log_group.log_group]
  volume {
    name      = "proc"
    host_path = "/proc"
  }
  volume {
    name      = "dev"
    host_path = "/dev"
  }
  volume {
    name      = "al1_cgroup"
    host_path = "/cgroup"
  }
  volume {
    name      = "al2_cgroup"
    host_path = "/sys/fs/cgroup"
  }
}

resource "aws_ecs_service" "cwagent_service" {
  name                = "cwagent-service-${module.common.testing_id}"
  cluster             = aws_ecs_cluster.cluster.id
  task_definition     = aws_ecs_task_definition.cwagent_task_definition.arn
  launch_type         = "EC2"
  scheduling_strategy = "DAEMON"
  enable_execute_command = true

  depends_on = [aws_ecs_task_definition.cwagent_task_definition]
}

#####################################################################
# Sample app for scrapping metrics and logs and sending to cloudwatch
#####################################################################

data "template_file" "extra_apps" {
  template = file(local.extra_apps_ecs_taskdef)
  vars = {
    region    = var.region
    log_group = aws_cloudwatch_log_group.log_group.name
  }
}

resource "aws_ecs_task_definition" "extra_apps_task_definition" {
  family                   = "extra-apps-family-${module.common.testing_id}"
  network_mode             = "awsvpc"
  task_role_arn            = module.basic_components.role_arn
  execution_role_arn       = module.basic_components.role_arn
  cpu                      = 256
  memory                   = 1024
  requires_compatibilities = ["FARGATE"]
  container_definitions    = data.template_file.extra_apps.rendered
  depends_on               = [aws_cloudwatch_log_group.log_group]
}

resource "aws_ecs_service" "extra_apps_service" {
  name            = "extra-apps-service-${module.common.testing_id}"
  cluster         = aws_ecs_cluster.cluster.id
  task_definition = aws_ecs_task_definition.extra_apps_task_definition.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  enable_execute_command = true

  network_configuration {
    security_groups  = [module.basic_components.security_group]
    subnets          = module.basic_components.public_subnet_ids
    assign_public_ip = true
  }

  depends_on = [aws_ecs_task_definition.extra_apps_task_definition]
}

resource "null_resource" "validator" {
  provisioner "local-exec" {
    command = <<-EOT
      echo Checking task metadata
      echo "Validating metrics/logs"
      cd ../../..
      go test ${var.test_dir} -timeout 0 -computeType=ECS -ecsLaunchType=EC2 -ecsDeploymentStrategy=DAEMON -cwagentConfigSsmParamName=${local.cwagent_config_ssm_param_name} -clusterArn=${aws_ecs_cluster.cluster.arn} -cwagentECSServiceName=${aws_ecs_service.cwagent_service.name} -v
    EOT
  }
  depends_on = [aws_ecs_service.cwagent_service, aws_ecs_service.extra_apps_service, null_resource.disable_metadata]
}

resource "null_resource" "disable_metadata" {

  provisioner "local-exec" {
    command = <<-EOT
      echo Sleep for 30 seconds to allow instance to be attached to the cluster
      sleep 30
      echo Setting metadata option for ECS EC2 instance to ${var.metadataEnabled}
      INSTANCEID=`aws ec2 describe-instances --filters Name=tag:ClusterName,Values=${aws_ecs_cluster.cluster.name} --query "Reservations[*].Instances[*].InstanceId" --output text`
      echo Instance ID for ECS instance is $INSTANCEID
      aws ec2 modify-instance-metadata-options --instance-id $INSTANCEID --http-endpoint ${var.metadataEnabled}
    EOT
  }
  depends_on = [aws_ecs_service.cwagent_service, aws_ecs_service.extra_apps_service, aws_autoscaling_group.cluster]
}