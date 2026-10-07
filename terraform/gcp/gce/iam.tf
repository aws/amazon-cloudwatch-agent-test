// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# CWAGENT_ROLE: assumed via web identity (GCP service-account identity token). Carries the agent's
# default:otel CloudWatch writes plus the reads the on-VM test needs to assert delivery.

module "common" {
  source = "../../common"
}

# Per-run service account: the identity the VM mints tokens as. It holds no GCP permissions: it
# exists only so the metadata server mints identity tokens for it, and its unique ID is what the
# role trust pins. account_id caps at 30 chars, hence the short prefix.
resource "google_service_account" "cwagent" {
  account_id   = "cwa-gce-${module.common.testing_id}"
  display_name = "cwa-gce-integ-${module.common.testing_id}"
}

data "aws_iam_policy_document" "cwagent_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    # Google is a built-in web-identity provider, so no IAM OIDC provider resource is involved:
    # the principal is accounts.google.com itself.
    principals {
      type        = "Federated"
      identifiers = ["accounts.google.com"]
    }

    # Pin all three Google condition keys -- the recommended trust policy for Google-issued tokens
    # and the same shape the onboarding scripts create. :sub is the service account's unique ID,
    # :aud reads the token's azp claim (the unique ID again on service-account tokens), and :oaud
    # is the audience the token was requested with, rejecting tokens minted for other services.
    # https://aws.amazon.com/blogs/security/access-aws-using-a-google-cloud-platform-native-workload-identity/
    condition {
      test     = "StringEquals"
      variable = "accounts.google.com:aud"
      values   = [google_service_account.cwagent.unique_id]
    }

    condition {
      test     = "StringEquals"
      variable = "accounts.google.com:sub"
      values   = [google_service_account.cwagent.unique_id]
    }

    condition {
      test     = "StringEquals"
      variable = "accounts.google.com:oaud"
      values   = [var.gcp_token_audience]
    }
  }
}

resource "aws_iam_role" "cwagent" {
  name               = "cwa-gce-integ-role-${module.common.testing_id}"
  assume_role_policy = data.aws_iam_policy_document.cwagent_assume_role.json
}

# The agent's own writes come from the same AWS-managed policy customers are told to use, so a green run
# also proves that documented policy is sufficient over the GCP web-identity path.
resource "aws_iam_role_policy_attachment" "cwagent_server_policy" {
  role       = aws_iam_role.cwagent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Validation reads only -- the agent's own writes are fully covered by CloudWatchAgentServerPolicy. The
# test binary runs on the VM under this same role, so these have to live here.
data "aws_iam_policy_document" "cwagent_permissions" {
  statement {
    effect = "Allow"
    actions = [
      "cloudwatch:ListMetrics",
      "cloudwatch:GetMetricData",
      "logs:GetLogEvents",
      # Cleanup: the test deletes its own stream from the shared /aws/cwagent/otlp group when it finishes.
      "logs:DeleteLogStream",
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
  name   = "cwa-gce-integ-policy-${module.common.testing_id}"
  role   = aws_iam_role.cwagent.id
  policy = data.aws_iam_policy_document.cwagent_permissions.json
}
