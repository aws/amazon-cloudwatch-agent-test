// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../common"
}

data "aws_iam_instance_profile" "cwagent_instance_profile" {
  name = module.common.cwa_iam_instance_profile
}

data "aws_iam_role" "cwagent_iam_role" {
  name = module.common.cwa_iam_role
}

data "aws_vpc" "vpc" {
  count   = var.create_ipv6_vpc ? 0 : 1
  default = var.vpc_name == "" ? true : null

  dynamic "filter" {
    for_each = var.vpc_name != "" ? [1] : []
    content {
      name   = "tag:Name"
      values = [var.vpc_name]
    }
  }
}

data "aws_subnets" "public_subnet_ids" {
  count = var.create_ipv6_vpc ? 0 : 1

  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.vpc[0].id]
  }
}

data "aws_security_group" "security_group" {
  count = var.create_ipv6_vpc ? 0 : 1
  name  = module.common.vpc_security_group
}

# Create security group for IPv6 VPC
resource "aws_security_group" "ipv6_security_group" {
  count = var.create_ipv6_vpc ? 1 : 0

  name        = "cwagent-ipv6-sg-${module.common.testing_id}"
  description = "Security group for CloudWatch Agent IPv6 testing"
  vpc_id      = aws_vpc.ipv6_vpc[0].id

  egress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  ingress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name = "cwagent-ipv6-sg-${module.common.testing_id}"
  }
}

# Local values to determine which VPC and subnets to use
locals {
  vpc_id            = var.create_ipv6_vpc ? aws_vpc.ipv6_vpc[0].id : data.aws_vpc.vpc[0].id
  subnet_ids        = var.create_ipv6_vpc ? aws_subnet.ipv6_public_subnet[*].id : data.aws_subnets.public_subnet_ids[0].ids
  security_group_id = var.create_ipv6_vpc ? aws_security_group.ipv6_security_group[0].id : data.aws_security_group.security_group[0].id
}

