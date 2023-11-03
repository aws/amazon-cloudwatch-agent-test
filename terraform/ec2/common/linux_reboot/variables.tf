// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "test_dir" {
  type    = string
  default = ""
}

variable "reboot_required_tests" {
  type    = list(any)
  default = []
}

variable "private_key_content" {
  type    = string
  default = ""
}

variable "cwagent_public_ip" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = ""
}
