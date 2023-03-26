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
  selected_ami             = var.family
  ami_family               = var.ami_family[local.selected_ami]
  temp_folder              = local.ami_family["temp_folder"]
  start_agent              = local.ami_family["start_command"]
  connection_type          = local.ami_family["connection_type"]
  agent_config_destination = local.ami_family["agent_config_destination"]
  test_dir                 = "${var.test_dir}/${local.selected_ami}"
  login_user               = lookup(local.selected_ami, "ec2-user", local.ami_family["login_user"])
  user_data                = lookup(local.selected_ami, "user_data", local.ami_family["user_data"])
  download_command         = format(local.ami_family["download_command_pattern"], "s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/${var.arc}/${local.ami_family["install_package"]}")
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
}

locals {
  download_validator = "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${local.selected_ami}/${var.arc}/validator ."
  prepare_validation = format("%s --validator-config=${local.temp_folder}/${local.final_validator_config} --preparation-mode=true", local.selected_ami == "windows" ? "validator.exe" : "validator")
  start_validation   = format("%s --validator-config=${local.temp_folder}/${local.final_validator_config} --preparation-mode=false", local.selected_ami == "windows" ? "validator.exe" : "validator")
}

resource "local_file" "update-validation-config" {
  content = replace(file("${local.test_dir}/${local.validator_config}"),
  "<cloudwatch_agent_config>", "${local.agent_config_destination}")

  filename = "${local.test_dir}/${local.final_validator_config}"
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
    Name = "cwagent-canary-${module.common.testing_id}"
  }
}

resource "null_resource" "prepare_validation" {
  depends_on = [aws_instance.cwagent, null_resource.upload-validator]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = "${local.test_dir}/${local.final_validator_config}"
    destination = "${local.temp_folder}/${local.final_validator_config}"
  }

  provisioner "file" {
    source      = "${local.test_dir}/${local.cloudwatch_agent_config}"
    destination = local.agent_config_destination
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      local.ami_family["wait_cloud_init"],
      local.download_command,
      local.download_validator,
      local.ami_family["install_command"],
    ]
  }
}

resource "null_resource" "windows_validation" {
  count      = local.selected_ami == "windows" ? 1 : 0
  depends_on = [aws_instance.cwagent, null_resource.upload-validator]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }

  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      local.prepare_validation,
      local.start_agent,
      local.start_validation,
    ]
  }
}

resource "null_resource" "linux_validation" {
  count      = local.selected_ami != "windows" ? 1 : 0
  depends_on = [aws_instance.cwagent, null_resource.upload-validator]

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
      "sudo chmod +x ./validator",
      local.prepare_validation,
      local.start_agent,
      local.start_validation,
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
