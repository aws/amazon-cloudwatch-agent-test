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