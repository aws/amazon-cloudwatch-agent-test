// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
}

resource "local_file" "update-validation-config" {
  content = replace(replace(replace(replace(file("${var.test_dir}/${local.validator_config}"),
    "<values_per_minute>", var.values_per_minute),
    "<commit_hash>", var.cwa_github_sha),
    "<commit_date>", var.cwa_github_sha_date),
  "<cloudwatch_agent_config>", "${var.instance_temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}


