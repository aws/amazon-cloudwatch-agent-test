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
  filter {
    name   = "tag:Name"
    values = [module.common.vpc]
  }
}

# return public subnets
data "aws_subnets" "public_subnet_ids" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.vpc.id]
  }

  filter {
    name = "tag:Name"
    values = [
      "${module.common.vpc}-public-${var.region}a",
      "${module.common.vpc}-public-${var.region}b",
      "${module.common.vpc}-public-${var.region}c",
    ]
  }
}

data "aws_security_group" "security_group" {
  name = module.common.vpc_security_group
}

resource "random_id" "subnet_selector" {
  byte_length = 2
}

locals {
  subnet_ids_list = tolist(data.aws_subnets.public_subnet_ids.ids)

  subnet_ids_random_index = random_id.subnet_selector.dec % length(local.subnet_ids_list)

  random_instance_subnet_id = local.subnet_ids_list[local.subnet_ids_random_index]
}

