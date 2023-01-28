// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../common"
}

locals {
  ssh_key_name            = var.ssh_key_name != "" ? var.ssh_key_name : aws_key_pair.aws_ssh_key[0].key_name
  private_key_content     = var.ssh_key_name != "" ? var.ssh_key_value : tls_private_key.ssh_key[0].private_key_pem
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
  test_dir                = "../../test/statsd_stress"
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
}

resource "local_file" "update-helm-config" {
  content  = replace(replace(file("${local.test_dir}/${local.validator_config}"), 
                "<data_rate>", var.data_rate),
                "<cloudwatch_agent_config>",local.cloudwatch_agent_config
            )
  filename = "${local.test_dir}/${local.final_validator_config}"

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

  tags = {
    Name = "cwagent-performance-${var.test_name}-${module.common.testing_id}"
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
    source      = "${local.test_dir}/${local.final_validator_config}"
    destination = "/tmp/${local.final_validator_config}"

    
  }

  provisioner "file" {
    source      = "${local.test_dir}/${local.cloudwatch_agent_config}"
    destination = "/tmp/${local.cloudwatch_agent_config}"
  }

  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "cloud-init status --wait",
      "echo clone and install agent",
      "rm -rf amazon-cloudwatch-agent-test",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
    ]
  }
  

  #Run sanity check and integration test
  provisioner "remote-exec" {
    inline = [
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "echo run integration test",
      "cd ~/amazon-cloudwatch-agent-test",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:/tmp/${local.cloudwatch_agent_config}",
      "go run ./validator/main.go --validator-config=/tmp/${local.final_validator_config}",
      "exit 1"
    ]
  }

  depends_on = [aws_instance.cwagent]
}

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "506463145083"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}

data "aws_dynamodb_table" "performance-dynamodb-table" {
  name = module.common.performance-dynamodb-table
}