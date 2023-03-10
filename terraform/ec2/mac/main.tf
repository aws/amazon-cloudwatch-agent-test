// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
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
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {
  ami                         = data.aws_ami.latest.id
  instance_type               = var.ec2_instance_type
  key_name                    = local.ssh_key_name
  iam_instance_profile        = data.aws_iam_instance_profile.cwagent_instance_profile.name
  vpc_security_group_ids      = [data.aws_security_group.ec2_security_group.id]
  associate_public_ip_address = true
  tenancy                     = "host"

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-${var.test_name}-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
    timeout     = "10m"
  }
  provisioner "remote-exec" {
    inline = [
      #Install AWS CLI
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
      "sudo installer -pkg ./amazon-cloudwatch-agent.pkg -target /",
    ]
  }

  depends_on = [
    aws_instance.cwagent,
  ]
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
