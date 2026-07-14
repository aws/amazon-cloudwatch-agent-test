// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# CWAGENT_ROLE: assumed via web identity (Azure managed-identity JWT), scoped to default:otel CloudWatch writes.

module "common" {
  source = "../common"
}

data "aws_caller_identity" "current" {}

# Referenced by ARN, not created here (created once out-of-band; may be auto-removed if unapproved).
data "aws_iam_openid_connect_provider" "azure" {
  arn = var.azure_oidc_provider_arn
}

locals {
  # Provider URL as an IAM condition key: drop scheme + trailing slash, keep internal slashes.
  oidc_condition_key = trimsuffix(trimprefix(data.aws_iam_openid_connect_provider.azure.url, "https://"), "/")
}

data "aws_iam_policy_document" "cwagent_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.azure.arn]
    }

    # Restrict to the requested audience to prevent confused-deputy token reuse.
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_condition_key}:aud"
      values   = [var.azure_token_audience]
    }
  }
}

resource "aws_iam_role" "cwagent" {
  name               = "cwa-azurevm-integ-role-${module.common.testing_id}"
  assume_role_policy = data.aws_iam_policy_document.cwagent_assume_role.json
}

# CloudWatch write perms for default:otel: metrics, logs (+ log-group/stream create), and X-Ray.
data "aws_iam_policy_document" "cwagent_permissions" {
  statement {
    effect = "Allow"
    actions = [
      "cloudwatch:PutMetricData",
      "logs:PutLogEvents",
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:DescribeLogGroups",
      "logs:DescribeLogStreams",
      "xray:PutTraceSegments",
      "xray:PutTelemetryRecords",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "cwagent" {
  name   = "cwa-azurevm-integ-policy-${module.common.testing_id}"
  role   = aws_iam_role.cwagent.id
  policy = data.aws_iam_policy_document.cwagent_permissions.json
}
