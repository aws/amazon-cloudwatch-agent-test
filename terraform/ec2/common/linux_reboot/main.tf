// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# Execute test
#####################################################################

## reboot when only needed
resource "null_resource" "integration_test_reboot" {
  count = contains(var.reboot_required_tests, var.test_dir) ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = var.private_key_content
    host        = var.cwagent_public_ip
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      "echo reboot instance",
      "sudo shutdown -r now &",
    ]
  }
}

resource "null_resource" "integration_test_wait" {
  count = contains(var.reboot_required_tests, var.test_dir) ? 1 : 0
  provisioner "local-exec" {
    command = "echo Sleeping for 3m after initiating instance restart && sleep 180"
  }
  depends_on = [
    null_resource.integration_test_reboot,
  ]
}