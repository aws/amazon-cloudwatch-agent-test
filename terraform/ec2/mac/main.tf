// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
}

#####################################################################
# Generate EC2 Key Pair for log in access to EC2
#####################################################################

resource "tls_private_key" "ssh_key" {
  count     = var.ssh_key_name == "" ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "aws_ssh_key" {
  count      = var.ssh_key_name == "" ? 1 : 0
  key_name   = "ec2-key-pair-${module.common.testing_id}"
  public_key = tls_private_key.ssh_key[0].public_key_openssh
}

locals {
  ssh_key_name        = var.ssh_key_name != "" ? var.ssh_key_name : aws_key_pair.aws_ssh_key[0].key_name
  private_key_content = var.ssh_key_name != "" ? var.ssh_key_value : tls_private_key.ssh_key[0].private_key_pem
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
  instance_temp_directory = "/tmp"
}

resource "local_file" "update-validation-config" {
  content = replace(file("${var.test_dir}/${local.validator_config}"),
  "<cloudwatch_agent_config>", "${local.instance_temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}

// Build and uploading the validator to spending less time in 
// and avoid memory issue in allocating memory 
resource "null_resource" "upload-validator" {
  provisioner "local-exec" {
    command = <<-EOT
    cd ../../.. 
    make validator-build
    aws s3 cp ./build/validator/darwin/${var.arc}/validator s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/darwin/${var.arc}/validator
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
  tenancy                     = "host"

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-mac-${var.test_name}-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent, null_resource.upload-validator]

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
    timeout     = "10m"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.final_validator_config}"
    destination = "${local.instance_temp_directory}/${local.final_validator_config}"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.cloudwatch_agent_config}"
    destination = "${local.instance_temp_directory}/${local.cloudwatch_agent_config}"
  }

  provisioner "remote-exec" {
    inline = [
      # Install AWS CLI
      "sudo softwareupdate --install-rosetta --agree-to-license",
      "sudo curl https://awscli.amazonaws.com/AWSCLIV2.pkg -o AWSCLIV2.pkg",
      "sudo installer -pkg AWSCLIV2.pkg -target /",
      #Install Golang
      "mkdir homebrew && curl -L https://github.com/Homebrew/brew/tarball/master | tar xz --strip 1 -C homebrew",
      "homebrew/bin/brew install go",
    ]
  }
  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "/usr/local/bin/aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/${var.arc}/amazon-cloudwatch-agent.pkg .",
      "/usr/local/bin/aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/darwin/${var.arc}/validator .",
      "sudo installer -pkg amazon-cloudwatch-agent.pkg -target /",
    ]
  }

  #Prepare the requirement before validation and validate the metrics/logs/traces
  provisioner "remote-exec" {
    inline = [
      "sudo chmod +x ./validator",
      "export AWS_REGION=${var.region}",
      "./validator --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=true",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:${local.instance_temp_directory}/${local.cloudwatch_agent_config}",
      "./validator --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }

}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }

  filter {
    name   = "architecture"
    values = [var.arc == "arm64" ? "arm64_mac" : "x86_64_mac"]
  }
}
