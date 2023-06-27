// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

data "aws_iam_instance_profile" "cwagent_instance_profile" {
  name = module.common.cwa_iam_instance_profile
}
