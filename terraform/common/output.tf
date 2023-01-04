// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "testing_id" {
  value = random_id.testing_id.hex
}

output "cwa_iam_role" {
  value = var.cwa_iam_role
}

output "cwa_iam_policy" {
  value = var.cwa_iam_policy
}

output "cwa_iam_instance_profile" {
  value = var.cwa_iam_instance_profile
}

output "ec2_key_pair" {
  value = var.ec2_key_pair
}

output "cwagent_image_repo" {
  value = var.cwagent_image_repo
}

output "cwagent_image_tag" {
  value = var.cwagent_image_tag
}

output "vpc_security_group" {
  value = var.vpc_security_group
}

output "performance-dynamodb-table" {
  value = ""
}