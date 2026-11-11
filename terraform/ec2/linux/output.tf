// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// The AMI this run launched, so the run log shows which image and owner account were used.
output "ami_id" {
  value = module.linux_common.ami_id
}

output "ami_owner_id" {
  value = module.linux_common.ami_owner_id
}
