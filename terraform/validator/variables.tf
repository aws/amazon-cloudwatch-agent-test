// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "test_dir" {
  type    = string
  default = ""
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "cwa_github_sha_date" {
  type    = string
  default = ""
}

variable "values_per_minute" {
  type    = number
  default = 10
}


variable "instance_temp_directory" {
  type    = string
  default = "/tmp"
}
