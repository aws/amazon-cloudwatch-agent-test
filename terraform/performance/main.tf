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

module "validator" {
  source = "../validator"

  arc                 = var.arc
  family              = "linux"
  action              = "upload"
  s3_bucket           = var.s3_bucket
  test_dir            = var.test_dir
  temp_directory      = "/tmp"
  cwa_github_sha      = var.cwa_github_sha
  cwa_github_sha_date = var.cwa_github_sha_date
  values_per_minute   = var.values_per_minute
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

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent, module.validator]

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/amazon-cloudwatch-agent.rpm .",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/linux/${var.arc}/validator .",
      "sudo rpm -Uvh ./amazon-cloudwatch-agent.rpm"
    ]
  }

  #Prepare the requirement before validation and validate the metrics/logs/traces
  provisioner "remote-exec" {
    inline = [
      "export AWS_REGION=${var.region}",
      "sudo chmod +x ./validator",
      "./validator --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:${module.validator.instance_agent_config}",
      "./validator --validator-config=${module.validator.instance_validator_config} --preparation-mode=false",
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
