// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
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
# Generate IAM Assume Role for Credentials
#####################################################################
resource "aws_iam_role" "assume_role" {
  name               = "cwa-integ-assume-role-${module.common.testing_id}"
  assume_role_policy = data.aws_iam_policy_document.assume_role_trust_policy.json
}

data "aws_iam_policy_document" "assume_role_trust_policy" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      identifiers = [module.basic_components.role_arn]
      type        = "AWS"
    }
  }
}

data "aws_iam_policy_document" "assume_role_policy_doc" {
  statement {
    effect = "Allow"
    actions = [
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
      "s3:GetObject",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "assume_role_policy" {
  name   = "cwa-integ-assume-role-policy-${module.common.testing_id}"
  policy = data.aws_iam_policy_document.assume_role_policy_doc.json
}

resource "aws_iam_role_policy_attachment" "assume_role_policy_attachment" {
  role       = aws_iam_role.assume_role.name
  policy_arn = aws_iam_policy.assume_role_policy.arn
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

  depends_on = [aws_iam_role.assume_role]
}

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
      # Git configurations for China region
      "${var.region == "cn-north-1" ? "echo 'Configuring Git settings for China region...'" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.sslBackend gnutls" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.postBuffer 524288000" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.lowSpeedLimit 0" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.lowSpeedTime 3600" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.window 1" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.depth 1" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.packSizeLimit 1g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.deltaCacheSize 1g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.windowMemory 1g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global core.packedGitLimit 1g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global core.packedGitWindowSize 1g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.postBuffer 10g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global http.maxRequestBuffer 10g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global https.postBuffer 10g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global https.maxRequestBuffer 10g" : ""}",
      "${var.region == "cn-north-1" ? "git config --global pack.threads 128" : ""}",
      "${var.region == "cn-north-1" ? "echo 'Git configurations complete'" : ""}",
      
      # Go proxy configurations for China region
      "${var.region == "cn-north-1" ? "echo 'Configuring Go proxy for China region...'" : ""}",
      "${var.region == "cn-north-1" ? "go env -w GO111MODULE=on" : ""}",
      "${var.region == "cn-north-1" ? "go env -w GOPROXY=https://goproxy.cn,direct" : ""}",
      "${var.region == "cn-north-1" ? "go env -w GOSUMDB=sum.golang.google.cn" : ""}",
      "${var.region == "cn-north-1" ? "echo 'Current Go proxy settings:' && go env GOPROXY" : ""}",
      "echo clone and install agent",
      "git clone --depth 1 --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      # S3 download with retry logic
      "echo 'Downloading from S3 with retry...'",
      "for i in {1..3}; do",
      "  echo \"S3 download attempt $i\"",
      "  if aws s3 cp s3://${local.binary_uri} .; then",
      "    echo 'S3 download successful'",
      "    ls -l amazon-cloudwatch-agent*",
      "    sleep 5",
      "    break",
      "  else",
      "    echo \"S3 download attempt $i failed\"",
      "    sleep 10",
      "  fi",
      "  if [ $i -eq 3 ]; then",
      "    echo 'All S3 download attempts failed'",
      "    exit 1",
      "  fi",
      "done",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
  }

  depends_on = [
    aws_instance.cwagent,
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
    inline = [
      "echo prepare environment",
      "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "echo run integration test",
      "cd ~/amazon-cloudwatch-agent-test",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      "echo assume role arn is ${aws_iam_role.assume_role.arn}",
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -assumeRoleArn=${aws_iam_role.assume_role.arn} -instanceId=${aws_instance.cwagent.id} -v"
    ]
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
