// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "windows" {
  source        = "../"
  windows_os_version = var.windows_os_version
}