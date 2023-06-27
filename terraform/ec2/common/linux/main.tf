// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../common"
}

module "basic_components" {
  source = "../../../basic_components"

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

# create a proxy instance for tests using proxy instsance
module "proxy_instance" {
  source              = "../../proxy"
  create              = length(regexall("proxy", var.test_dir)) > 0 ? 1 : 0
  ssh_key_name        = local.ssh_key_name
  private_key_content = local.private_key_content
  test_dir            = var.test_dir
  test_name           = var.test_name
  region              = var.region
  user                = var.user
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

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = var.is_canary ? "cwagent-canary-test-ec2-${var.test_name}-${module.common.testing_id}" : "cwagent-integ-test-ec2-${var.test_name}-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test_fips_setup" {
  # run go test when it's not feature test
  count = length(regexall("fips", var.test_dir)) > 0 ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo enabling fips",
      "sudo yum install -y dracut-fips",
      "sudo dracut -f",
      "sudo /sbin/grubby --update-kernel=ALL --args=\"fips=1\"",
      "sudo reboot &",
    ]
  }

  depends_on = [
    aws_instance.cwagent,
  ]
}

resource "null_resource" "integration_test_fips_check" {
  # run go test when it's not feature test
  count = length(regexall("fips", var.test_dir)) > 0 ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo `cat /proc/sys/crypto/fips_enabled`",
      "echo `sysctl crypto.fips_enabled`",
      "if [ `cat /proc/sys/crypto/fips_enabled` -ne \"1\" ];then echo \"FIPS is not enabled, please check file /proc/sys/crypto/fips_enabled.\";exit 1; fi",
    ]
  }

  depends_on = [
    null_resource.integration_test_fips_setup,
  ]
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}
