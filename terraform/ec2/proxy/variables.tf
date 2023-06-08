// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ec2_instance_type" {
  type    = string
  default = "t3a.micro"
}

variable "ami" {
  type    = string
  default = "amzn2-ami-kernel-5.10-hvm*"
}

variable "create" {
  type    = number
  default = 0
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "private_key_content" {
  type    = string
  default = ""
}

variable "test_name" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = ""
}
