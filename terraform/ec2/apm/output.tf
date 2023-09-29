// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "private_key_content" {
  value = local.private_key_content
  sensitive = true
}

output "cwagent_public_ip" {
  value = aws_instance.cwagent.public_ip
}

output "cwagent_id" {
  value = aws_instance.cwagent.id
}