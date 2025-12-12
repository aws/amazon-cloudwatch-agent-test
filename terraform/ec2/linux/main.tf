// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT



module "linux_common" {
  source = "../common/linux"

  region            = var.region
  ec2_instance_type = var.ec2_instance_type
  ssh_key_name      = var.ssh_key_name
  ami               = var.ami
  ssh_key_value     = var.ssh_key_value
  user              = var.user
  arc               = var.arc
  test_name         = var.test_name
  test_dir          = var.test_dir
  is_canary         = var.is_canary
}

module "reboot_common" {
  source = "../common/linux_reboot"

  test_dir              = var.test_dir
  reboot_required_tests = local.reboot_required_tests
  private_key_content   = module.linux_common.private_key_content
  cwagent_public_ip     = module.linux_common.cwagent_public_ip
  user                  = var.user

  depends_on = [
    null_resource.integration_test_setup,
  ]
}

locals {
  // Canary downloads latest binary. Integration test downloads binary connect to git hash.
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
  // list of test that require instance reboot
  reboot_required_tests = tolist(["./test/restart"])

  // On-premises specific configuration
  is_onprem = var.is_onprem

  // Pre-test setup command
  pre_test_setup_cmd = local.is_onprem ? "echo 'Pre-test setup: Replacing {instance_id} and $${aws:InstanceId} placeholders in test resource configs'; find . -path '*/resources/*.json' -exec sed -i 's/{instance_id}/${module.linux_common.cwagent_id}/g' {} \\; -exec sed -i 's/$${aws:InstanceId}/${module.linux_common.cwagent_id}/g' {} \\; && echo 'Updated all config files in resources directories'" : var.pre_test_setup
}

#####################################################################
# Execute tests
#####################################################################

resource "null_resource" "integration_test_setup" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = module.linux_common.private_key_content
    host        = module.linux_common.cwagent_public_ip
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = concat(
      [
        "echo sha ${var.cwa_github_sha}",
        "sudo cloud-init status --wait",
        "echo clone ${var.github_test_repo} branch ${var.github_test_repo_branch} and install agent",
        # check for vendor directory specifically instead of overall test repo to avoid issues with SELinux
        "if [ ! -d amazon-cloudwatch-agent-test/vendor ]; then",
        "echo 'Vendor directory (test repo dependencies) not found, cloning...'",
        "sudo rm -r amazon-cloudwatch-agent-test",
        "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
        "else",
        "echo 'Test repo already exists, skipping clone'",
        "fi",
        "cd amazon-cloudwatch-agent-test",
        "git rev-parse --short HEAD",
        "aws s3 cp --no-progress s3://${local.binary_uri} .",
        "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      ],

      # On-premises specific setup
      local.is_onprem ? [
        "sudo mkdir -p ~/.aws",
        "echo creating credentials file that the agent uses by default for onprem",
        "printf '[default]\\nregion = us-west-2\\n' | sudo tee ~/.aws/config",
        "echo attempting to assume role for on-premises credentials",
        "ASSUME_ROLE_OUTPUT=$(aws sts assume-role --role-arn ${module.linux_common.cwa_onprem_assumed_iam_role_arm} --role-session-name onpremtest --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text)",
        "if [ $? -ne 0 ]; then echo 'Failed to assume role'; exit 1; fi",
        "echo 'Creating default credentials'",
        "printf '[default]\\naws_access_key_id=%s\\naws_secret_access_key=%s\\naws_session_token=%s\\n' $ASSUME_ROLE_OUTPUT | sudo tee ~/.aws/credentials>/dev/null",
        "echo verifying credentials are working",
        "aws sts get-caller-identity || echo 'Credentials test failed'",
        "echo turning off imds access in order to make agent start with onprem mode",
        "aws ec2 modify-instance-metadata-options --instance-id ${module.linux_common.cwagent_id} --http-endpoint disabled",
        "echo waiting for IMDS to be fully disabled",
        "sleep 10",
        "sudo mkdir -p /opt/aws/amazon-cloudwatch-agent/etc",
        "printf '[credentials]\\n  shared_credential_profile = \"default\"\\n  shared_credential_file = \"/home/ubuntu/.aws/credentials\"\\n' | sudo tee /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml>/dev/null",
        "echo setting environment variables for agent",
        "echo 'RUN_IN_AWS=false' | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/env-config",
        "echo 'INSTANCE_ID=${module.linux_common.cwagent_id}' | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/env-config",
        "echo 'export RUN_IN_AWS=false' | sudo tee -a /etc/environment",
        "echo 'export INSTANCE_ID=${module.linux_common.cwagent_id}' | sudo tee -a /etc/environment",
      ] : [],

      [
        var.install_agent,
      ]
    )
  }

  depends_on = [
    module.linux_common,
    null_resource.download_test_repo_and_vendor_from_s3,
  ]
}

# Download vendor directory and cloned test repo from S3 for CN region tests
resource "null_resource" "download_test_repo_and_vendor_from_s3" {
  # set to only run in CN region
  count = startswith(var.region, "cn-") ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = module.linux_common.private_key_content
    host        = module.linux_common.cwagent_public_ip
  }
  provisioner "remote-exec" {
    inline = [
      "echo Downloading cloned test repo from S3...",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/cloudwatch-agent-test-repo/${var.cwa_github_sha}.tar.gz ./amazon-cloudwatch-agent-test.tar.gz --quiet",
      "mkdir amazon-cloudwatch-agent-test",
      "tar -xzf amazon-cloudwatch-agent-test.tar.gz -C amazon-cloudwatch-agent-test",
      "cd amazon-cloudwatch-agent-test",
      "export GO111MODULE=on",
      "export GOFLAGS=-mod=vendor",
      "echo 'Vendor directory copied from S3'"
    ]
  }

  depends_on = [
    module.linux_common,
  ]
}

module "amp" {
  count                    = length(regexall("/amp", var.test_dir)) > 0 ? 1 : 0
  source                   = "terraform-aws-modules/managed-service-prometheus/aws"
  workspace_alias          = "cwagent-integ-test-${module.linux_common.testing_id}"
  retention_period_in_days = 7
  limits_per_label_set     = []
}

resource "null_resource" "integration_test_run" {
  connection {
    type        = "ssh"
    user        = var.user
    private_key = module.linux_common.private_key_content
    host        = module.linux_common.cwagent_public_ip
  }

  provisioner "remote-exec" {
    inline = concat(
      [
        "echo Preparing environment...",
        "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
      ],

      # SELinux test setup (if enabled)
      var.is_selinux_test ? [
        "sudo yum install amazon-cloudwatch-agent -y",
        "echo Running SELinux test setup...",
        "sudo yum install selinux-policy selinux-policy-targeted policycoreutils-python-utils selinux-policy-devel -y",
        "sudo setenforce 1",
        "echo below is either Permissive/Enforcing",
        "sudo getenforce",
        "sudo rm -r amazon-cloudwatch-agent-selinux",
        "git clone --branch ${var.selinux_branch} https://github.com/aws/amazon-cloudwatch-agent-selinux.git",
        "cd amazon-cloudwatch-agent-selinux",
        "cat amazon_cloudwatch_agent.te",
        "chmod +x ./amazon_cloudwatch_agent.sh",
        "sudo ./amazon_cloudwatch_agent.sh -y",
        ] : [
        "echo SELinux test not enabled"
      ],

      # General testing setup
      [
        "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
        "export AWS_REGION=${var.region}",
        "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      ],

      [
        "echo Running integration test...",
        "cd ~/amazon-cloudwatch-agent-test",
      ],

      # On-premises specific environment variables
      local.is_onprem ? [
        "export RUN_IN_AWS=false",
        "export AWS_EC2_METADATA_DISABLED=true",
        "export AWS_PROFILE=default",
        "export AWS_SHARED_CREDENTIALS_FILE=~/.aws/credentials",
        "export AWS_CONFIG_FILE=~/.aws/config",
        "echo 'Environment variables for on-premises test:'",
        "echo 'AWS_REGION='$AWS_REGION",
        "echo 'RUN_IN_AWS='$RUN_IN_AWS",
        "echo 'AWS_EC2_METADATA_DISABLED='$AWS_EC2_METADATA_DISABLED",
        "echo 'AWS_PROFILE='$AWS_PROFILE",
        "echo 'Instance ID parameter: ${module.linux_common.cwagent_id}'",
        "echo 'Testing AWS credentials:'",
        "aws sts get-caller-identity || echo 'AWS credentials test failed'",
        "echo 'Testing agent credentials:'",
        "sudo aws sts get-caller-identity || echo 'Agent credentials test failed'",
        "echo 'Pre-test setup: Replacing {instance_id} and $${aws:InstanceId} placeholders in test resource configs'; find . -path '${var.test_dir}/resources/*.json' -exec sed -i 's/{instance_id}/${module.linux_common.cwagent_id}/g' {} \\; -exec sed -i 's/$${aws:InstanceId}/${module.linux_common.cwagent_id}/g' {} \\; && echo 'Updated all config files in resources directories'"
      ] : [
        "echo Running sanity test...",
        "go test ./test/sanity -p 1 -v",
      ],

      [
        var.pre_test_setup,
        # Integration test execution with conditional agent start command
        "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -excludedTests='${var.excluded_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -proxyUrl=${module.linux_common.proxy_instance_proxy_ip} -instanceId=${module.linux_common.cwagent_id} ${local.is_onprem ? "-agentStartCommand='${var.agent_start}'" : ""} ${length(regexall("/amp", var.test_dir)) > 0 ? "-ampWorkspaceId=${module.amp[0].workspace_id} " : ""}-v"
      ],
    )
  }

  depends_on = [
    null_resource.integration_test_setup,
    null_resource.download_test_repo_and_vendor_from_s3,
    module.reboot_common,
  ]
}