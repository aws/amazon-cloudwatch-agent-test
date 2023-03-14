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
  ssh_key_name        = var.ssh_key_name != "" ? var.ssh_key_name : aws_key_pair.aws_ssh_key[0].key_name
  private_key_content = var.ssh_key_name != "" ? var.ssh_key_value : tls_private_key.ssh_key[0].private_key_pem
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
  content = replace(replace(replace(replace(file("${var.test_dir}/${local.validator_config}"),
    "<values_per_minute>", var.values_per_minute),
    "<commit_hash>", var.cwa_github_sha),
    "<commit_date>", var.cwa_github_sha_date),
  "<cloudwatch_agent_config>", "${local.instance_temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"

}

#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {
  ami                         = data.aws_ami.latest.id
  instance_type               = var.ec2_instance_type
  key_name                    = local.ssh_key_name
  iam_instance_profile        = module.basic_components.instance_profile
  subnet_id                   = module.basic_components.random_subnet_instance_id
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

resource "null_resource" "integration_test" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  # Prepare Integration Test
  provisioner "file" {
    source      = "${var.test_dir}/${local.final_validator_config}"
    destination = "${local.instance_temp_directory}/${local.final_validator_config}"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.cloudwatch_agent_config}"
    destination = "${local.instance_temp_directory}/${local.cloudwatch_agent_config}"
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/amazon-cloudwatch-agent.rpm .",
      "sudo rpm -U ./amazon-cloudwatch-agent.rpm"
    ]
  }

  #Prepare the requirement before validation and validate the metrics/logs/traces
  provisioner "remote-exec" {
    inline = [
      "export AWS_REGION=${var.region}",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd ~/amazon-cloudwatch-agent-test",
      "go run ./validator/main.go --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=true",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:${local.instance_temp_directory}/${local.cloudwatch_agent_config}",
      "go run ./validator/main.go --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }

  depends_on = [aws_instance.cwagent]
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
