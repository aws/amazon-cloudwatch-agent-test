// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# create vpc with nat gateway so that we can use it to launch awsvpc ecs task in both ecs and fargate
module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name = module.common.vpc
  cidr = "10.0.0.0/16"

  azs             = ["${var.region}a", "${var.region}b", "${var.region}c"]
  private_subnets = ["10.0.0.0/19", "10.0.32.0/19", "10.0.64.0/19"]
  public_subnets  = ["10.0.128.0/19", "10.0.160.0/19", "10.0.192.0/19"]

  enable_nat_gateway = true
  enable_vpn_gateway = true

  enable_dns_hostnames = true
  enable_dns_support   = true
}

resource "aws_security_group" "ec2_security_group" {
  name   = module.common.vpc_security_group
  vpc_id = module.vpc.vpc_id

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

  // Allow access to EFS. https://docs.aws.amazon.com/efs/latest/ug/troubleshooting-efs-mounting.html#mount-hangs-fails-timeout
  egress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "TCP"
    cidr_blocks = ["0.0.0.0/0"]
  }


  // Default ECS Prometheus 
  // https://github.com/aws/amazon-cloudwatch-agent-test/blob/d5105cdc461c6fcb13049cf2d38c287674d94e21/terraform/ecs/linux/default_resources/default_extra_apps.tpl
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

  // NFS https://docs.aws.amazon.com/efs/latest/ug/troubleshooting-efs-mounting.html#mount-hangs-fails-timeout
  ingress {
    from_port   = 2049
    to_port     = 2049
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
}
