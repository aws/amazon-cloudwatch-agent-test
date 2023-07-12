// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "private_key_content" {
  value = local.private_key_content
}

output "cwagent_public_ip" {
  value = aws_instance.cwagent.public_ip
}

output "cwagent_id" {
  value = aws_instance.cwagent.id
}

output "proxy_instance_proxy_ip" {
  value = module.proxy_instance.proxy_ip
}

output "cwa_onprem_assumed_iam_role_arm" {
  value = module.common.cwa_onprem_assumed_iam_role_arm
}