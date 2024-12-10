// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

resource "random_id" "testing_id" {
  byte_length = 8
}

data "http" "myip" {
  url = "http://icanhazip.com"
}

locals {
  my_ip = chomp(data.http.myip.response_body)
}
data "aws_ec2_managed_prefix_list" "existing_list" {
  id = var.prefix_list_id
}

# Then, create the entry only if it doesn't already exist
resource "aws_ec2_managed_prefix_list_entry" "prefix_list_entry" {
  count          = contains(data.aws_ec2_managed_prefix_list.existing_list.entries[*].cidr, "${local.my_ip}/32") ? 0 : 1
  prefix_list_id = var.prefix_list_id
  cidr           = "${local.my_ip}/32"
}
