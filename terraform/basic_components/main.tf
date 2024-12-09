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
  default = true
}

data "aws_subnets" "public_subnet_ids" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.vpc.id]
  }
}

data "aws_security_group" "security_group" {
  name = module.common.vpc_security_group
}

data "http" "myip" {
  url = "http://icanhazip.com"
}

locals {
  my_ip = chomp(data.http.myip.response_body)
}

resource "null_resource" "manage_ip_in_prefix_list" {
  triggers = {
    run_id = module.common.testing_id
    prefix_list_id = module.common.runner_prefix_list_id
    my_ip = local.my_ip
  }

  # Add or update the IP in the prefix list on apply
  provisioner "local-exec" {
    command = "/bin/bash ../../../basic_components/add_ip.sh ${self.triggers.prefix_list_id} ${self.triggers.my_ip}"
  }

  # Remove the IP from the prefix list on destroy
  provisioner "local-exec" {
    when = destroy
    command = "/bin/bash ../../../basic_components/remove_ip.sh ${self.triggers.prefix_list_id} ${self.triggers.my_ip}"
  }
  
}
