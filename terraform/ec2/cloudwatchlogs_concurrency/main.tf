// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
}

locals {
  iam_role_name = "cwa-concurrency-${module.common.testing_id}"
}

#####################################################################
# Generate EC2 Key Pair for log in access to EC2
#####################################################################

resource "tls_private_key" "ssh_key" {
  count     = var.ssh_key_name == "" ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "aws_ssh_key" {
  count      = var.ssh_key_name == "" ? 1 : 0
  key_name   = "ec2-key-pair-${module.common.testing_id}"
  public_key = tls_private_key.ssh_key[0].public_key_openssh
}

locals {
  ssh_key_name        = var.ssh_key_name != "" ? var.ssh_key_name : aws_key_pair.aws_ssh_key[0].key_name
  private_key_content = var.ssh_key_name != "" ? var.ssh_key_value : tls_private_key.ssh_key[0].private_key_pem
  binary_uri          = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
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
        Sid    = "DenyRestrictedLogGroups"
        Effect = "Deny"
        Action = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "arn:aws:logs:*:*:log-group:aws-restricted-log-group-name-*"
      },
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
# Generate EC2 Instance
#####################################################################

resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = aws_iam_instance_profile.cwagent_instance_profile.name
  vpc_security_group_ids               = [module.basic_components.security_group]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-${var.test_name}-${module.common.testing_id}"
  }
}

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "amazon"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}

#####################################################################
# Test Setup and Execution
#####################################################################

resource "null_resource" "integration_test_setup" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "sudo cloud-init status --wait",
      "echo clone ${var.github_test_repo} branch ${var.github_test_repo_branch} and install agent",
      "if [ ! -d amazon-cloudwatch-agent-test/vendor ]; then",
      "echo 'Vendor directory not found, cloning...'",
      "sudo rm -rf amazon-cloudwatch-agent-test",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
      "fi",
      "cd amazon-cloudwatch-agent-test",
      "git rev-parse --short HEAD",
      "aws s3 cp --no-progress s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
  }

  depends_on = [
    aws_iam_role.cwagent_role,
    aws_iam_role_policy.cwagent_policy
  ]
}

resource "null_resource" "integration_test_run" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    inline = [
      "echo Preparing environment...",
      "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "cd ~/amazon-cloudwatch-agent-test",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${aws_instance.cwagent.id} -iamRoleName=${local.iam_role_name} -v"
    ]
  }

  depends_on = [null_resource.integration_test_setup]
}
