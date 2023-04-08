// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "testing_id" {
  value = random_id.testing_id.hex
}

output "cloudwatch_agent_config" {
  value = "${var.test_dir}/${local.cloudwatch_agent_config}"
}

output "instance_cloudwatch_agent_config" {
  value = "${var.instance_temp_directory}/${local.cloudwatch_agent_config}"
}

output "validator_config" {
  value = "${var.test_dir}/${local.final_validator_config}"
}

output "instance_validator_config" {
  value = "${var.instance_temp_directory}/${local.cloudwatch_agent_config}"
}

