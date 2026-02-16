// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// This module wraps terraform/ec2/linux and adds a per-test IAM role
// with self-modify permissions for the recovery test.

module "common" {
  source = "../../common"
}

locals {
  iam_role_name = "cwa-concurrency-${module.common.testing_id}"
}

#####################################################################
# Per-test IAM Role with self-modify permissions
#####################################################################

resource "aws_iam_role" "cwagent_role" {
  name = local.iam_role_name

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "cwagent_policy" {
  name = "${local.iam_role_name}-policy"
  role = aws_iam_role.cwagent_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "cloudwatch:PutMetricData",
          "cloudwatch:GetMetricData",
          "cloudwatch:ListMetrics",
          "logs:PutLogEvents",
          "logs:DescribeLogStreams",
          "logs:DescribeLogGroups",
          "logs:CreateLogStream",
          "logs:CreateLogGroup",
          "logs:DeleteLogGroup",
          "logs:DeleteLogStream",
          "logs:PutRetentionPolicy",
          "logs:GetLogEvents",
          "ec2:DescribeVolumes",
          "ec2:DescribeTags",
          "ec2:DescribeInstances",
          "ssm:GetParameter",
          "ssm:Describe*",
          "ssm:Get*",
          "ssm:List*",
          "s3:GetObject",
          "s3:GetObjectAcl",
          "s3:ListBucket",
        ]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = ["iam:PutRolePolicy", "iam:DeleteRolePolicy"]
        Resource = aws_iam_role.cwagent_role.arn
      }
    ]
  })
}

resource "aws_iam_instance_profile" "cwagent_instance_profile" {
  name = "${local.iam_role_name}-profile"
  role = aws_iam_role.cwagent_role.name
}

#####################################################################
# Use the standard linux module with custom IAM
#####################################################################

module "linux_common" {
  source = "../common/linux"

  region               = var.region
  ec2_instance_type    = var.ec2_instance_type
  ssh_key_name         = var.ssh_key_name
  ami                  = var.ami
  ssh_key_value        = var.ssh_key_value
  user                 = var.user
  arc                  = var.arc
  test_name            = var.test_name
  test_dir             = var.test_dir
  is_canary            = var.is_canary
  iam_instance_profile = aws_iam_instance_profile.cwagent_instance_profile.name
}

locals {
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
}

#####################################################################
# Test Setup and Execution
#####################################################################

resource "null_resource" "integration_test_setup" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = module.linux_common.private_key_content
    host        = module.linux_common.cwagent_public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "sudo cloud-init status --wait",
      "echo clone ${var.github_test_repo} branch ${var.github_test_repo_branch}",
      "if [ ! -d amazon-cloudwatch-agent-test/vendor ]; then",
      "sudo rm -rf amazon-cloudwatch-agent-test",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
      "fi",
      "cd amazon-cloudwatch-agent-test",
      "aws s3 cp --no-progress s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
  }

  depends_on = [module.linux_common]
}

resource "null_resource" "integration_test_run" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = module.linux_common.private_key_content
    host        = module.linux_common.cwagent_public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "cd ~/amazon-cloudwatch-agent-test",
      "go test ./test/sanity -p 1 -v",
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${module.linux_common.cwagent_id} -iamRoleName=${local.iam_role_name} -v"
    ]
  }

  depends_on = [null_resource.integration_test_setup]
}
