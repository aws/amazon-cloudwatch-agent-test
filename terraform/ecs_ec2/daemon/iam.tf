// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

data "aws_iam_role" "ecs_task_role" {
  name = module.common.cwa_iam_role
}

data "aws_iam_role" "ecs_task_execution_role" {
  name = module.common.cwa_iam_role
}

data "aws_iam_instance_profile" "cwagent_instance_profile" {
  name = module.common.cwa_iam_instance_profile
}