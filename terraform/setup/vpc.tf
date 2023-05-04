// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

data "aws_vpc" "default" {
  default = true
}

resource "aws_security_group" "ec2_security_group" {
  name   = module.common.vpc_security_group
  vpc_id = data.aws_vpc.default.id

  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // Allow access to IMDS
  egress {
    from_port   = 80
    to_port     = 80
    protocol    = "TCP"
    cidr_blocks = ["169.254.169.254/32"]
  }

  // Default ECS Prometheus
  // https://github.com/aws/amazon-cloudwatch-agent-test/blob/d5105cdc461c6fcb13049cf2d38c287674d94e21/terraform/ecs/linux/default_resources/default_extra_apps.tpl
  egress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 9121
    to_port     = 9121
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }


  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 9121
    to_port     = 9121
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // OpenSSH and others ssh into EC2 Instance
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // localstack http and https
  ingress {
    from_port   = 4566
    to_port     = 4566
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // WinRM http https://developer.hashicorp.com/terraform/language/resources/provisioners/connection#argument-reference
  ingress {
    from_port   = 5985
    to_port     = 5985
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // WinRM https https://developer.hashicorp.com/terraform/language/resources/provisioners/connection#argument-reference
  ingress {
    from_port   = 5986
    to_port     = 5986
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }

  // RDP https://en.wikipedia.org/wiki/Remote_Desktop_Protocol
  ingress {
    from_port   = 3389
    to_port     = 3389
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}