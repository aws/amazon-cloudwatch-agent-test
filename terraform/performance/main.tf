// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../common"
}

module "basic_components" {
  source = "../basic_components"

  region = var.region
}

locals {
  ami_family        = var.ami_family[var.family]
  login_user        = local.ami_family["login_user"]
  install_package   = local.ami_family["install_package"]
  install_validator = local.ami_family["install_validator"]
  temp_directory    = local.ami_family["temp_folder"]
  connection_type   = local.ami_family["connection_type"]
  start_command     = format(local.ami_family["start_command"], "${local.temp_directory}/${local.cloudwatch_agent_config}")
}


#####################################################################
# Prepare Parameters Tests
#####################################################################

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
  "<cloudwatch_agent_config>", "${local.temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}

// Build and uploading the validator to spending less time in 
// and avoid memory issue in allocating memory 
resource "null_resource" "upload-validator" {
  provisioner "local-exec" {
    command = <<-EOT
    cd ../..
    make validator-build
    aws s3 cp ./build/validator/${var.family}/${var.arc}/validator s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${var.family}/${var.arc}/validator
    EOT
  }

  triggers = {
    always_run = "${timestamp()}"
  }
}

#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {

  ami                         = data.aws_ami.latest.id
  instance_type               = var.ec2_instance_type
  key_name                    = local.ssh_key_name
  iam_instance_profile        = module.basic_components.instance_profile
  vpc_security_group_ids      = [module.basic_components.security_group]
  associate_public_ip_address = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-performance-${module.common.testing_id}"
  }
}

resource "null_resource" "install_binaries" {
  depends_on = [aws_instance.cwagent, null_resource.upload-validator]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.final_validator_config}"
    destination = "${local.temp_directory}/${local.final_validator_config}"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.cloudwatch_agent_config}"
    destination = "${local.temp_directory}/${local.cloudwatch_agent_config}"
  }

  provisioner "remote-exec" {
    inline = [
      local.ami_family["wait_cloud_init"],
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/${var.family}/${var.arc}/${local.install_package} .",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${var.family}/${var.arc}/${local.install_validator} .",
      local.ami_family["install_command"],
    ]
  }
}

resource "null_resource" "validator_linux" {
  count      = var.family != "windows" ? 1 : 0
  depends_on = [aws_instance.cwagent, null_resource.upload-validator, null_resource.install_binaries]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }
  provisioner "remote-exec" {
    inline = [
      "export AWS_REGION=${var.region}",
      "sudo chmod +x ./${local.install_validator}",
      "./${local.install_validator} --validator-config=${local.temp_directory}/${local.final_validator_config} --preparation-mode=true",
      local.start_command,
      "./${local.install_validator} --validator-config=${local.temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }
}

resource "null_resource" "validator_windows" {
  count      = var.family == "windows" ? 1 : 0
  depends_on = [aws_instance.cwagent, null_resource.upload-validator, null_resource.install_binaries]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
    timeout     = "10m"
  }

  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "${local.install_validator} --validator-config=${local.temp_directory}/${local.final_validator_config} --preparation-mode=true",
      local.start_command,
      "${local.install_validator} --validator-config=${local.temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}

data "aws_dynamodb_table" "performance-dynamodb-table" {
  name = module.common.performance-dynamodb-table
}
