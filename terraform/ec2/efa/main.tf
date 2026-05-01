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
# SSH Key
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
  binary_uri          = "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
}

#####################################################################
# AMI Lookup
#####################################################################

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "amazon"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}

#####################################################################
# EFA Security Group
#####################################################################

resource "aws_security_group" "efa" {
  name_prefix = "efa-integ-test-${module.common.testing_id}-"
  vpc_id      = module.basic_components.vpc_id

  # EFA requires all traffic within the security group
  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    self      = true
  }

  # SSH access
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # All outbound
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "efa-integ-test-${module.common.testing_id}"
  }
}

#####################################################################
# EFA Placement Group
#####################################################################

resource "aws_placement_group" "efa" {
  name     = "efa-integ-test-${module.common.testing_id}"
  strategy = "cluster"
}

#####################################################################
# EFA Network Interface
#####################################################################

resource "aws_network_interface" "efa" {
  subnet_id       = module.basic_components.public_subnet_ids[0]
  security_groups = [aws_security_group.efa.id]
  interface_type  = "efa"

  tags = {
    Name = "efa-integ-test-${module.common.testing_id}"
  }
}

#####################################################################
# Elastic IP for SSH access (network_interface block does not support
# associate_public_ip_address)
#####################################################################

resource "aws_eip" "efa" {
  domain            = "vpc"
  network_interface = aws_network_interface.efa.id

  tags = {
    Name = "efa-integ-test-${module.common.testing_id}"
  }

  depends_on = [aws_instance.cwagent]
}

#####################################################################
# EC2 Instance with EFA
#####################################################################

resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
  placement_group                      = aws_placement_group.efa.name
  instance_initiated_shutdown_behavior = "terminate"

  network_interface {
    device_index         = 0
    network_interface_id = aws_network_interface.efa.id
  }

  user_data = <<-EOT
    #!/bin/bash
    sudo sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
    sudo sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config
    sudo sed -i 's/^#*KbdInteractiveAuthentication.*/KbdInteractiveAuthentication no/' /etc/ssh/sshd_config
    sudo systemctl restart sshd
  EOT

  root_block_device {
    volume_size = 200
  }

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-efa-${var.test_name}-${module.common.testing_id}"
  }
}

#####################################################################
# EFA Driver Installation + Test Setup
#####################################################################

resource "null_resource" "integration_test_setup" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_eip.efa.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "sudo cloud-init status --wait",

      "# Install EFA driver",
      "cd /tmp",
      "curl -O https://efa-installer.amazonaws.com/aws-efa-installer-latest.tar.gz",
      "tar -xf aws-efa-installer-latest.tar.gz",
      "cd aws-efa-installer",
      "sudo ./efa_installer.sh -y",
      "echo 'EFA driver installation complete'",
      "fi_info -p efa 2>/dev/null || { echo 'FATAL: EFA provider not available'; exit 1; }",
      "ls /sys/class/infiniband/ || { echo 'FATAL: No EFA device found at /sys/class/infiniband/'; exit 1; }",

      "# Clone test repo and install agent",
      "cd ~",
      "echo clone ${var.github_test_repo} branch ${var.github_test_repo_branch} and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
      "cd amazon-cloudwatch-agent-test",
      "git rev-parse --short HEAD",
      "aws s3 cp --no-progress s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
  }

  depends_on = [
    aws_instance.cwagent,
  ]
}

#####################################################################
# Run Integration Test
#####################################################################

resource "null_resource" "integration_test_run" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_eip.efa.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "cd ~/amazon-cloudwatch-agent-test",
      "echo Running sanity test...",
      "go test ./test/sanity -p 1 -v",
      "echo Running EFA integration test...",
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -excludedTests='${var.excluded_tests}' -cwaCommitSha=${var.cwa_github_sha} -instanceId=${aws_instance.cwagent.id} -v",
    ]
  }

  depends_on = [
    null_resource.integration_test_setup,
  ]
}
