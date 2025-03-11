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
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "sudo cloud-init status --wait",
      "echo clone and install agent",
      "sudo rm -r amazon-cloudwatch-agent-test",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "git rev-parse --short HEAD",
      "aws s3 cp --no-progress s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]
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
  count           = length(regexall("/amp", var.test_dir)) > 0 ? 1 : 0
  source          = "terraform-aws-modules/managed-service-prometheus/aws"
  workspace_alias = "cwagent-integ-test-${module.linux_common.testing_id}"
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
        "sudo setenforce 0",
        "echo enforcing mode on",
        "sudo yum install -y audit policycoreutils-python-utils go --allowerasing",
        "sudo systemctl start auditd",
        "sudo systemctl enable auditd",
      ],

      # SELinux test setup (if enabled)
      var.is_selinux_test ? [
        "sudo yum install amazon-cloudwatch-agent -y",
        "echo Running SELinux test setup...",
        "sudo yum install selinux-policy selinux-policy-targeted selinux-policy-devel -y",
        "sudo setenforce 0",
        "echo below is either Permissive/Enforcing",
        "sudo getenforce",
        "sudo rm -r amazon-cloudwatch-agent-selinux",
        "git clone --branch ${var.selinux_branch} https://github.com/aws/amazon-cloudwatch-agent-selinux.git",
        "cd amazon-cloudwatch-agent-selinux",
        "cat amazon_cloudwatch_agent.te",
        "chmod +x ./amazon_cloudwatch_agent.sh",
        "sudo ./amazon_cloudwatch_agent.sh",
        ] : [
        "echo SELinux test not enabled"
      ],

      # General testing setup
      [
        "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
        "export AWS_REGION=${var.region}",
        "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
        "echo Running integration test...",
        "cd ~/amazon-cloudwatch-agent-test",
        "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
        "echo Running sanity test...",
        "go test ./test/sanity -p 1 -v",
        var.pre_test_setup,
        # Integration test execution
        "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -excludedTests='${var.excluded_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -proxyUrl=${module.linux_common.proxy_instance_proxy_ip} -instanceId=${module.linux_common.cwagent_id} ${length(regexall("/amp", var.test_dir)) > 0 ? "-ampWorkspaceId=${module.amp[0].workspace_id} " : ""}-v",
        "sudo ausearch -m AVC,USER_AVC -ts recent | audit2allow -M custom_policy",
        "cat custom_policy.te"
      ],
    )
  }

  depends_on = [
    null_resource.integration_test_setup,
    null_resource.download_test_repo_and_vendor_from_s3,
    module.reboot_common,
  ]
}