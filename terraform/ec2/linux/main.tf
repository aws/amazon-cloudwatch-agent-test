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
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "aws s3 cp s3://${local.binary_uri} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
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

  #Run sanity check and integration test
  provisioner "remote-exec" {
    inline = [
      "echo prepare environment",
      "sudo setenforce 1",
      "sudo rm -r amazon-cloudwatch-agent-sepolicy",
      "${var.is_selinux_test ? "sudo setenforce 1" : "echo SELinux not enforced"}",
      "${var.is_selinux_test ? "for i in {1..3}; do git clone https://github.com/Paramadon/amazon-cloudwatch-agent-sepolicy.git && break || sleep 5; done" : "echo SELinux test not enabled"}",
      "${var.is_selinux_test ? "cd amazon-cloudwatch-agent-sepolicy && sudo chmod +x amazon_cloudwatch_agent.sh && sudo ./amazon_cloudwatch_agent.sh" : "echo Skipping SELinux setup"}",
      "sleep 10",
      "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "echo run integration test",
      "cd ~/amazon-cloudwatch-agent-test",
      "nohup bash -c 'while true; do sudo shutdown -c; sleep 30; done' >/dev/null 2>&1 &",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      var.pre_test_setup,
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -excludedTests='${var.excluded_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -proxyUrl=${module.linux_common.proxy_instance_proxy_ip} -instanceId=${module.linux_common.cwagent_id} ${length(regexall("/amp", var.test_dir)) > 0 ? "-ampWorkspaceId=${module.amp[0].workspace_id} " : ""}-v",
    ]
  }

  depends_on = [
    null_resource.integration_test_setup,
    module.reboot_common,
  ]
}
