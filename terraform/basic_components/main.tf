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
    run_id         = module.common.testing_id
    prefix_list_id = module.common.runner_prefix_list_id
    my_ip          = local.my_ip
  }

  # Add or update the IP in the prefix list on apply
  provisioner "local-exec" {
    command = <<-EOT
      #!/bin/bash
      set -e
      PREFIX_LIST_ID=${self.triggers.prefix_list_id}
      MY_IP=${self.triggers.my_ip}
      CURRENT_VERSION=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Version' --output text)
      EXISTING_IPS=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Entries[*].Cidr' --output text)
      if echo "$EXISTING_IPS" | grep -q "$MY_IP/32"; then
        echo "$MY_IP/32 is already present in the prefix list."
      else
        aws ec2 modify-managed-prefix-list \
          --prefix-list-id "$PREFIX_LIST_ID" \
          --current-version "$CURRENT_VERSION" \
          --add-entries Cidr="$MY_IP/32",Description="Added by Terraform"
        echo "Successfully added $MY_IP/32 to the prefix list."
      fi
    EOT
  }

  # Remove the IP from the prefix list on destroy
  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      #!/bin/bash
      set -e
      PREFIX_LIST_ID=${self.triggers.prefix_list_id}
      MY_IP=${self.triggers.my_ip}
      CURRENT_VERSION=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Version' --output text)
      EXISTING_IPS=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Entries[*].Cidr' --output text)
      if echo "$EXISTING_IPS" | grep -q "$MY_IP/32"; then
        aws ec2 modify-managed-prefix-list \
          --prefix-list-id "$PREFIX_LIST_ID" \
          --current-version "$CURRENT_VERSION" \
          --remove-entries Cidr="$MY_IP/32"
        echo "Successfully removed $MY_IP/32 from the prefix list."
      else
        echo "$MY_IP/32 is not present in the prefix list."
      fi
    EOT
  }
}
