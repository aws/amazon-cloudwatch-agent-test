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
      "export LOCAL_STACK_HOST_NAME=${var.local_stack_host_name}",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "echo run integration test",
      "cd ~/amazon-cloudwatch-agent-test",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      var.pre_test_setup,
      "go test ${var.test_dir} -p 1 -timeout 1h -computeType=EC2 -bucket=${var.s3_bucket} -plugins='${var.plugin_tests}' -excludedTests='${var.excluded_tests}' -cwaCommitSha=${var.cwa_github_sha} -caCertPath=${var.ca_cert_path} -proxyUrl=${module.linux_common.proxy_instance_proxy_ip} -instanceId=${module.linux_common.cwagent_id} -v",
    ]
  }

  depends_on = [
    null_resource.integration_test_setup,
    module.reboot_common,
  ]
}
