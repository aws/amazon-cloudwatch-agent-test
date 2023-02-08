// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_security_group" "ec2_security_group" {
  name = module.common.vpc_security_group
}

resource "aws_network_interface" "test" {
  subnet_id       = data.aws_subnets.default.ids[0]
  security_groups = [data.aws_security_group.ec2_security_group.id]

  attachment {
    instance     = aws_instance.cwagent.id
    device_index = 1
  }
}