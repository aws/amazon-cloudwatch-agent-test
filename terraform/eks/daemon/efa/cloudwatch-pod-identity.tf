// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# IAM role for CloudWatch Observability addon
resource "aws_iam_role" "cloudwatch_observability" {
  name = "cloudwatch-observability-${module.common.testing_id}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "pods.eks.amazonaws.com"
        }
        Action = [
          "sts:AssumeRole",
          "sts:TagSession"
        ]
      }
    ]
  })

  tags = {
    Name  = "cloudwatch-observability-${module.common.testing_id}"
    Owner = "default"
  }
}

# Attach CloudWatch policies
resource "aws_iam_role_policy_attachment" "cloudwatch_observability_server" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.cloudwatch_observability.name
}