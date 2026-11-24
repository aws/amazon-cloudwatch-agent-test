// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// The AMI this run launched, so the run log shows which image and owner account were used.
output "ami_id" {
  value = data.aws_ami.latest.id
}

output "ami_owner_id" {
  value = data.aws_ami.latest.owner_id
}
