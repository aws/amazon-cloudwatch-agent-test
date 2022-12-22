// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}


variable "test_dir" {
  type    = string
  default = "./test/ecs/ecs_metadata"
}