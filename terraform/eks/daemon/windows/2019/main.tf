// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "windows" {
  source        = "../"
  windows_ami_type = var.windows_ami_type
  windows_os_version = var.windows_os_version
  test_dir = var.test_dir
  ami_type = var.ami_type
  instance_type = var.instance_type
  k8s_version = var.k8s_version
}