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


resource "aws_security_group" "proxy_sg" {
  count = var.create
  name        = "cwagent-proxy-${module.common.testing_id}"
  description = "communication with proxy instance"
  vpc_id      = module.basic_components.vpc_id
}

resource "aws_security_group_rule" "proxy_inbound" {
  description              = "Allow proxy port"
  from_port                = 3128
  to_port                  = 3128
  protocol                 = "tcp"
  security_group_id        = aws_security_group.proxy_sg.id
  type                     = "ingress"
}

resource "aws_security_group_rule" "proxy_outbound" {
  description              = "Allow cluster API Server to communicate with the worker nodes"
  from_port                = 1024
  protocol                 = "tcp"
  security_group_id        = aws_security_group.proxy_sg.id
  to_port                  = 65535
  type                     = "egress"
}

#####################################################################
# Generate proxy EC2 Instance
#####################################################################
resource "aws_instance" "cwintegproxy" {
  count = var.create
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = var.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
  vpc_security_group_ids               = [module.basic_components.security_group, aws_security_group.proxy_sg.id]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-${var.test_name}-${module.common.testing_id}"
  }

  depends_on = [
    aws_security_group.proxy_sg
  ]
}

resource "null_resource" "integration_test_proxy_setup" {
  count = var.create

  connection {
    type        = "ssh"
    user        = var.user
    private_key = var.private_key_content
    host        = aws_instance.cwintegproxy.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "yum install squid -y",
      "sed -i 's/net.ipv4.ip_forward.*/net.ipv4.ip_forward = 1/g' /etc/sysctl.conf",
      "setenforce 0",
      "sed -i 's/http_port.*/http_port 3128/g' /etc/squid/squid.conf",
      "squid –k parse",
      "service squid start"
    ]
  }

  depends_on = [
    aws_instance.cwintegproxy,
  ]
}

output "proxy_ip" {
  value = aws_instance.cwintegproxy.public_ip
}

output "proxy_dns_name" {
  value = aws_instance.cwintegproxy.public_dns
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}
