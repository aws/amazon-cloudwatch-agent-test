// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
}

data "aws_caller_identity" "account_id" {}

output "account_id" {
  value = data.aws_caller_identity.account_id.account_id
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
  // Canary downloads latest binary. Integration test downloads binary connect to git hash.
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
}


#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
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

#####################################################################
# Generate IAM Roles for Credentials
#####################################################################

locals {
  roles = {
    no_context_keys = {
      suffix    = ""
      condition = {}
    }
    source_arn_key = {
      suffix = "-source_arn_key"
      condition = {
        "aws:SourceArn" = aws_instance.cwagent.arn
      }
    }
    source_account_key = {
      suffix = "-source_account_key"
      condition = {
        "aws:SourceAccount" = data.aws_caller_identity.account_id.account_id
      }
    }
    all_context_keys = {
      suffix = "-all_context_keys"
      condition = {
        "aws:SourceArn"     = aws_instance.cwagent.arn
        "aws:SourceAccount" = data.aws_caller_identity.account_id.account_id
      }
    }
  }
}

resource "aws_iam_role" "roles" {
  for_each = local.roles

  name = "cwa-integ-assume-role-${module.common.testing_id}${each.value.suffix}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = module.basic_components.role_arn
        }
        Condition = length(each.value.condition) > 0 ? {
          StringEquals = each.value.condition
        } : {}
      }
    ]
  })
}

resource "aws_iam_role_policy" "cloudwatch_policy" {
  for_each = aws_iam_role.roles

  name = "${each.value.name}_policy"
  role = each.value.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "cloudwatch:PutMetricData",
          "cloudwatch:ListMetrics",
          "cloudwatch:GetMetricStatistics",
          "cloudwatch:GetMetricData",
          "logs:PutRetentionPolicy",
          "logs:PutLogEvents",
          "logs:GetLogEvents",
          "logs:DescribeLogStreams",
          "logs:DescribeLogGroups",
          "logs:DeleteLogStream",
          "logs:DeleteLogGroup",
          "logs:CreateLogStream",
          "logs:CreateLogGroup",
          "ssm:List*",
          "ssm:Get*",
          "ssm:Describe*",
          "s3:PutObject",
          "s3:ListBucket",
          "s3:GetObjectAcl",
          "s3:GetObject"
        ]
        Effect   = "Allow"
        Resource = "*"
      }
    ]
  })
}

#####################################################################
# Run the integration test
#####################################################################

resource "null_resource" "integration_test_setup" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "sudo cloud-init status --wait",
      "echo clone and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "aws s3 cp s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
  }

  depends_on = [
    aws_iam_role.roles,
    aws_iam_role_policy.cloudwatch_policy
  ]
}

resource "null_resource" "integration_test_run" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  #Run sanity check and integration test
  provisioner "remote-exec" {


    inline = concat(
      [
        "echo Preparing environment...",
        "sudo yum install -y audit policycoreutils-python-utils go --allowerasing",
      ],

      # SELinux test setup (if enabled)
      var.is_selinux_test ? [
        "sudo setenforce 1",
        "echo Running SELinux test setup...",
        "git clone --branch ${var.selinux_branch} https://github.com/aws/amazon-cloudwatch-agent-selinux.git",
        "cd amazon-cloudwatch-agent-selinux",
        "sudo chmod +x amazon_cloudwatch_agent.sh",
        "sudo ./amazon_cloudwatch_agent.sh -y"
        ] : [
        "echo SELinux test not enabled"
      ],

      # General testing setup
      [
        "export AWS_REGION=${var.region}",
        "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
        "echo run integration test",
        "cd ~/amazon-cloudwatch-agent-test",
        "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
        "echo run sanity test && go test ./test/sanity -p 1 -v",
        "echo base assume role arn is ${aws_iam_role.roles["no_context_keys"].arn}",
        "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -assumeRoleArn=${aws_iam_role.roles["no_context_keys"].arn} -instanceArn=${aws_instance.cwagent.arn} -accountId=${data.aws_caller_identity.account_id.account_id} -v"
      ],
    )
  }

  depends_on = [
    null_resource.integration_test_setup,
  ]
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}


