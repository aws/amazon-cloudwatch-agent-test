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
  // Canary downloads latest binary. Integration test downloads binary connect to git hash.
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
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
  user_data                            = data.template_file.init.rendered

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
  }

  #Run sanity check and integration test
  provisioner "remote-exec" {
    inline = concat(
      [
        "echo Getting Cloud-init Logs",
        "sudo cat /var/log/cloud-init-output.log",
      ],

      # SELinux test setup (if enabled)
      var.is_selinux_test ? [
        "sudo setenforce 1",
        "echo Running SELinux test setup...",
        "git clone --branch ${var.selinux_branch} https://github.com/aws/amazon-cloudwatch-agent-selinux.git",
        "cd amazon-cloudwatch-agent-selinux",
        "sudo chmod +x amazon_cloudwatch_agent.sh",
        "sudo ./amazon_cloudwatch_agent.sh -y"
        ] : [
        "echo SELinux test not enabled"
      ],

      # General testing setup
      [
        "echo prepare environment",
        "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
        "export AWS_REGION=${var.region}",
        "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
        "echo run integration test",
        "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
        "echo waiting for test directory...",
        "timeout 120 bash -c 'until [ -f /home/ec2-user/amazon-cloudwatch-agent-test/test/sanity/resources/verifyUnixCtlScript.sh ]; do echo \"Waiting for verifyUnixCtlScript.sh...\"; sleep 2; done'",
        "cd ~/amazon-cloudwatch-agent-test",
        "sudo chmod 777 ~/amazon-cloudwatch-agent-test/test/sanity/resources/verifyUnixCtlScript.sh",
        "echo run sanity test && go test ./test/sanity -p 1 -v",
        "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -v"
      ],
    )
  }

  depends_on = [
    aws_instance.cwagent,
  ]
}

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "amazon"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}

#####################################################################
# Generate template file for EC2 userdata script
#####################################################################
data "template_file" "init" {
  template = file("install_and_start_agent.sh")

  vars = {
    cwa_github_sha          = var.cwa_github_sha
    github_test_repo_branch = var.github_test_repo_branch
    github_test_repo        = var.github_test_repo
    binary_uri              = local.binary_uri
    install_agent           = var.install_agent
  }
}