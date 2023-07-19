// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
}

resource "local_file" "update-validation-config" {
  content = replace(replace(replace(replace(replace(file("${var.test_dir}/${local.validator_config}"),
    "<values_per_minute>", var.values_per_minute),
    "<commit_hash>", var.cwa_github_sha),
    "<commit_date>", var.cwa_github_sha_date),
    "<os_family>", var.family),
  "<cloudwatch_agent_config>", "${var.temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}

resource "null_resource" "build-validator" {
  provisioner "local-exec" {
    command     = var.action == "upload" ? "make validator-build" : "make dockerized-build"
    working_dir = split("test", var.test_dir)[0]
  }

  triggers = {
    always_run = "${timestamp()}"
  }
}

// Uploading the validator to spending less time in 
// and avoid memory issue in allocating memory with Windows
// Use case: EC2
resource "null_resource" "upload-validator" {
  count      = var.action == "upload" ? 1 : 0
  depends_on = [null_resource.build-validator]

  provisioner "local-exec" {
    command = (
      var.family == "windows" ?
      "aws s3 cp ./build/validator/${var.family}/${var.arc}/validator.exe s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${var.family}/${var.arc}/validator.exe" :
      "aws s3 cp ./build/validator/${var.family}/${var.arc}/validator s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${var.family}/${var.arc}/validator"
    )
    working_dir = split("test", var.test_dir)[0]
  }

  triggers = {
    always_run = "${timestamp()}"
  }
}

// Running the validator locally instead of uploading to EC2.
// Use case: ECS, EKS
resource "null_resource" "run-validator" {
  count      = var.action == "validate" ? 1 : 0
  depends_on = [null_resource.build-validator]

  provisioner "local-exec" {
    command = ""
  }

  triggers = {
    always_run = "${timestamp()}"
  }
}
