// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# CWAGENT_ROLE: assumed via web identity (Azure managed-identity JWT), scoped to default:otel CloudWatch writes.

module "common" {
  source = "../../common"
}

data "aws_caller_identity" "current" {}

# Referenced by ARN, not created here (created once out-of-band; may be auto-removed if unapproved).
data "aws_iam_openid_connect_provider" "azure" {
  arn = var.azure_oidc_provider_arn
}

locals {
  # Provider URL as an IAM condition key: drop the scheme but keep everything else verbatim —
  # Azure AD issuers end in "/" and IAM condition keys retain it (sts.windows.net/<tenant>/:aud).
  oidc_condition_key = trimprefix(data.aws_iam_openid_connect_provider.azure.url, "https://")
}

data "aws_iam_policy_document" "cwagent_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.azure.arn]
    }

    # The audience is a shared Azure resource that any identity in the tenant can request a token for,
    # so it is not sufficient on its own. Pin :sub to this VM's system-assigned identity principal so
    # only this VM can assume the role (mirrors the :sub scoping on the AKS service account).
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_condition_key}:sub"
      values   = [azurerm_linux_virtual_machine.cwagent.identity[0].principal_id]
    }

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

# Agent writes (default:otel) plus the reads the on-VM test binary needs to validate delivery.
data "aws_iam_policy_document" "cwagent_permissions" {
  statement {
    effect = "Allow"
    actions = [
      "cloudwatch:PutMetricData",
      "cloudwatch:ListMetrics",
      "cloudwatch:GetMetricData",
      "logs:PutLogEvents",
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:DescribeLogGroups",
      "logs:DescribeLogStreams",
      "logs:GetLogEvents",
      # StartQuery/GetQueryResults let the test binary validate OTLP trace delivery via the aws/spans
      # log group. That group is only populated where the X-Ray trace segment destination is set to
      # CloudWatchLogs, which is a per-region setting -- hence the region default in variables.tf.
      "logs:StartQuery",
      "logs:GetQueryResults",
      "xray:PutSpans",
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
