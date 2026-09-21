// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "role_arn" {
  value = aws_iam_role.cwagent.arn
}
