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
# Generate EC2 Instance and execute test commands
#####################################################################

resource "aws_instance" "integration-test" {
  ami                    = data.aws_ami.latest.id
  instance_type          = var.ec2_instance_type
  key_name               = local.ssh_key_name
  iam_instance_profile   = module.basic_components.instance_profile
  vpc_security_group_ids = [module.basic_components.security_group]

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  provisioner "remote-exec" {
    inline = concat(
      # Run these commands first in the CN regions (downloads test repo from S3 for StartLocalStackCN step)
      startswith(var.region, "cn-") ?
      [
        "echo Downloading cloned test repo from S3...",
        "aws s3 cp s3://${var.s3_bucket}/integration-test/cloudwatch-agent-test-repo/${var.cwa_github_sha}.tar.gz ./amazon-cloudwatch-agent-test.tar.gz --quiet",
        "mkdir amazon-cloudwatch-agent-test",
        "tar -xzf amazon-cloudwatch-agent-test.tar.gz -C amazon-cloudwatch-agent-test",
        "echo Downloading LocalStack image from S3...",
        "aws s3 cp s3://${var.s3_bucket}/integration-test/docker-images/${var.cwa_github_sha}.tar .",
        "docker load < ${var.cwa_github_sha}.tar",
        "rm ${var.cwa_github_sha}.tar",
      ] : [],
      # Common steps for all regions
      [
        "cloud-init status --wait",
        "echo clone the agent and start the localstack",
        "if [ ! -d amazon-cloudwatch-agent-test ]; then",
        "echo 'Test repo not found, cloning...'",
        "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
        "else",
        "echo 'Test repo already exists, skipping clone.'",
        "fi",
        "cd amazon-cloudwatch-agent-test",
        "git reset --hard ${var.cwa_test_github_sha}",
        "echo set up ssl pem for localstack, then start localstack",
        "cd ~/amazon-cloudwatch-agent-test/localstack/ls_tmp",
        "openssl req -new -x509 -newkey rsa:2048 -sha256 -nodes -out snakeoil.pem -keyout snakeoil.key -config snakeoil.conf",
        "cat snakeoil.key snakeoil.pem > server.test.pem",
        "cat snakeoil.key > server.test.pem.key",
        "cat snakeoil.pem > server.test.pem.crt",
        "cd ~/amazon-cloudwatch-agent-test/localstack",
        "docker-compose up -d --force-recreate",
        "aws s3 cp ls_tmp s3://${var.s3_bucket}/integration-test/ls_tmp/${var.cwa_github_sha} --recursive"
    ])
    connection {
      type        = "ssh"
      user        = "ec2-user"
      private_key = local.private_key_content
      host        = self.public_dns
    }
  }

  tags = {
    Name = "LocalStackIntegrationTestInstance"
  }
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = ["cloudwatch-agent-integration-test-aarch64-al2023*"]
  }
}
