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

module "validator" {
  source = "../../validator"

  arc            = var.arc
  family         = "darwin"
  action         = "upload"
  s3_bucket      = var.s3_bucket
  test_dir       = var.test_dir
  temp_directory = "/tmp"
  cwa_github_sha = var.cwa_github_sha
}

#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
  vpc_security_group_ids               = [module.basic_components.security_group]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"
  tenancy                              = "host"
  host_resource_group_arn              = var.license_manager_arn

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-mac-${var.test_name}-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent, module.validator]

  connection {
    type        = "ssh"
    user        = "ec2-user"
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
    timeout     = "10m"
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  provisioner "remote-exec" {
    inline = [
      # Install AWS CLI
      "sudo softwareupdate --install-rosetta --agree-to-license",
      "sudo curl https://awscli.amazonaws.com/AWSCLIV2.pkg -o AWSCLIV2.pkg",
      "sudo installer -pkg AWSCLIV2.pkg -target /",

      #Install Brew and set path for mac1 and mac2 instances
      "NONINTERACTIVE=1 /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"",
      "(echo; echo 'eval \"$(/usr/local/bin/brew shellenv)\"') >> /Users/ec2-user/.zprofile",
      "eval \"$(/usr/local/bin/brew shellenv)\"",
      "(echo; echo 'eval \"$(/opt/homebrew/bin/brew shellenv)\"') >> /Users/ec2-user/.zprofile",
      "eval \"$(/opt/homebrew/bin/brew shellenv)\"",

      #Download test repo
      "NONINTERACTIVE=1 brew install git",
      "echo clone test repo",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",

      # Install agent and validator binaries
      "echo Install agent and validator binaries",
      "/usr/local/bin/aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/${var.arc}/amazon-cloudwatch-agent.pkg .",
      "/usr/local/bin/aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/darwin/${var.arc}/validator .",
      "sudo installer -pkg amazon-cloudwatch-agent.pkg -target /",

      # Install Golang
      "echo Install golang",
      "NONINTERACTIVE=1 brew install go",

      # Run Integration test
      "echo Execute integration tests",
      "export AWS_REGION=${var.region}",
      "sudo chmod +x ./validator",
      "./validator --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:${module.validator.instance_agent_config}",
      "./validator --validator-config=${module.validator.instance_validator_config} --preparation-mode=false",
      "cd ~/amazon-cloudwatch-agent-test",
      "sudo go test ./test/run_as_user -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${aws_instance.cwagent.id} -v",
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

resource "null_resource" "integration_test_wait" {
  provisioner "local-exec" {
    command = <<-EOT
      echo "Sleeping after initiating instance restart"
      sleep 180
    EOT
  }
}
