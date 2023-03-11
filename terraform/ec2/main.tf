// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

locals {
  selected_ami             = var.family
  ami_family               = var.ami_family[local.selected_ami]
  connection_type          = local.ami_family["connection_type"]
  agent_config_destination = local.ami_family["agent_config_destination"]
  login_user               = lookup(local.selected_ami, "ec2-user", local.ami_family["login_user"])
  user_data                = lookup(local.selected_ami, "user_data", local.ami_family["user_data"])
  download_command         = format(local.ami_family["download_command_pattern"], "https://${var.package_s3_bucket}.s3.amazonaws.com/${local.selected_ami["os_family"]}/${local.selected_ami["arch"]}/${var.aoc_version}/${local.ami_family["install_package"]}")
  // Canary downloads latest binary. Integration test downloads binary connect to git hash.
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
}

#####################################################################
# Create EFS
#####################################################################
resource "aws_efs_file_system" "efs" {
  count          = local.selected_ami == "" ? 1 : 0
  creation_token = "efs-${module.common.testing_id}"
  tags = {
    Name = "efs-${module.common.testing_id}"
  }
}

resource "aws_efs_mount_target" "mount" {
  count           = local.selected_ami == "" ? 1 : 0
  file_system_id  = aws_efs_file_system.efs.id
  subnet_id       = aws_instance.cwagent.subnet_id
  security_groups = [data.aws_security_group.ec2_security_group.id]
}

resource "null_resource" "mount_efs" {
  count = local.selected_ami == "" ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    # https://docs.aws.amazon.com/efs/latest/ug/mounting-fs-mount-helper-ec2-linux.html
    inline = [
      "sudo mkdir ~/efs-mount-point",
      "sudo mount -t efs -o tls ${aws_efs_file_system.efs.dns_name} ~/efs-mount-point/",
    ]
  }

  depends_on = [
    aws_efs_mount_target.mount,
    aws_instance.cwagent
  ]
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

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      local.ami_family["wait_cloud_init"],
      local.download_command,
      local.ami_family["install_command"],
    ]
  }

  #Run sanity check and integration test
  provisioner "remote-exec" {
    inline = [
      "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "cd ~/amazon-cloudwatch-agent-test",
      "go test ./test/sanity -p 1 -v",
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -v"
    ]
  }

  depends_on = [
    aws_instance.cwagent,
    null_resource.mount_efs
  ]
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}
