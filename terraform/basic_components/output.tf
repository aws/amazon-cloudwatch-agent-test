// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "vpc_id" {
  value = local.vpc_id
}

output "security_group" {
  value = local.security_group_id
}

output "public_subnet_ids" {
  value = local.subnet_ids
}

output "role_arn" {
  value = data.aws_iam_role.cwagent_iam_role.arn
}

output "instance_profile" {
  value = data.aws_iam_instance_profile.cwagent_instance_profile.name
}


