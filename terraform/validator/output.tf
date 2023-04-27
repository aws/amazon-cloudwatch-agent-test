// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "agent_config" {
  value = "${var.test_dir}/${local.cloudwatch_agent_config}"
}

output "validator_config" {
  value = "${var.test_dir}/${local.final_validator_config}"
}

output "instance_agent_config" {
  value = "${var.temp_directory}/${local.cloudwatch_agent_config}"
}

output "instance_validator_config" {
  value = "${var.temp_directory}/${local.final_validator_config}"
}


