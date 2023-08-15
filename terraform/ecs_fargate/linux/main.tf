// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

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
  name = "cwagent-integ-test-cluster-${module.common.testing_id}"
}

resource "aws_cloudwatch_log_group" "log_group" {
  name = "cwagent-integ-test-log-group-${module.common.testing_id}"
}

##########################################
# Template Files
##########################################

locals {
  cwagent_config         = fileexists("../../../${var.test_dir}/resources/config.json") ? "../../../${var.test_dir}/resources/config.json" : "./default_resources/default_amazon_cloudwatch_agent.json"
  cwagent_ecs_taskdef    = fileexists("../../../${var.test_dir}/resources/ecs_taskdef.tpl") ? "../../../${var.test_dir}/resources/ecs_taskdef.tpl" : "./default_resources/default_ecs_taskdef.tpl"
  prometheus_config      = fileexists("../../../${var.test_dir}/resources/ecs_prometheus.tpl") ? "../../../${var.test_dir}/resources/ecs_prometheus.tpl" : "./default_resources/default_ecs_prometheus.tpl"
  extra_apps_ecs_taskdef = fileexists("../../../${var.test_dir}/resources/extra_apps.tpl") ? "../../../${var.test_dir}/resources/extra_apps.tpl" : "./default_resources/default_extra_apps.tpl"
}

data "template_file" "cwagent_config" {
  template = file(local.cwagent_config)
  vars = {
  }
}

resource "aws_ssm_parameter" "cwagent_config" {
  name  = "cwagent-integ-test-ssm-config-${module.common.testing_id}"
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
  network_mode             = "awsvpc"
  task_role_arn            = module.basic_components.role_arn
  execution_role_arn       = module.basic_components.role_arn
  cpu                      = 256
  memory                   = 2048
  requires_compatibilities = ["FARGATE"]
  container_definitions    = data.template_file.cwagent_container_definitions.rendered
  depends_on               = [aws_cloudwatch_log_group.log_group]
}

resource "aws_ecs_service" "cwagent_service" {
  name            = "cwagent-service-${module.common.testing_id}"
  cluster         = aws_ecs_cluster.cluster.id
  task_definition = aws_ecs_task_definition.cwagent_task_definition.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  enable_execute_command = true

  network_configuration {
    security_groups  = [module.basic_components.security_group]
    subnets          = module.basic_components.public_subnet_ids
    assign_public_ip = true
  }

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
      echo "Validating metrics/logs"
      cd ../../..
      go test ${var.test_dir} -clusterName=${aws_ecs_cluster.cluster.name} -v
    EOT
  }
  depends_on = [aws_ecs_service.cwagent_service, aws_ecs_service.extra_apps_service]
}
