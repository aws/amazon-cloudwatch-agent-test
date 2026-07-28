// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# CWAGENT_ROLE: assumed via web identity (Azure managed-identity JWT), scoped to default:otel CloudWatch writes.

module "common" {
  source = "../../common"
}

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

# The agent's own writes come from the same AWS-managed policy customers are told to use, so a green run
# also proves that documented policy is sufficient over the Azure web-identity path.
resource "aws_iam_role_policy_attachment" "cwagent_server_policy" {
  role       = aws_iam_role.cwagent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Everything CloudWatchAgentServerPolicy does not cover. The test binary runs on the VM under this same
# role, so the validation reads have to live here too.
data "aws_iam_policy_document" "cwagent_permissions" {
  statement {
    effect = "Allow"
    actions = [
      # Agent write omitted from CloudWatchAgentServerPolicy: the X-Ray OTLP endpoint needs PutSpans,
      # which is a different action from PutTraceSegments.
      "xray:PutSpans",
      # Reads used to assert delivery.
      "cloudwatch:ListMetrics",
      "cloudwatch:GetMetricData",
      "logs:GetLogEvents",
      # StartQuery/GetQueryResults validate OTLP trace delivery via the aws/spans log group. That group
      # is only populated where the X-Ray trace segment destination is set to CloudWatchLogs, which is a
      # per-region setting -- hence the region default in variables.tf.
      "logs:StartQuery",
      "logs:GetQueryResults",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "cwagent" {
  name   = "cwa-azurevm-integ-policy-${module.common.testing_id}"
  role   = aws_iam_role.cwagent.id
  policy = data.aws_iam_policy_document.cwagent_permissions.json
}
