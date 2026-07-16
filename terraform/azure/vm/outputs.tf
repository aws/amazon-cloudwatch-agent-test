// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "cwagent_public_ip" {
  value = azurerm_public_ip.cwagent.ip_address
}

output "cwagent_vm_id" {
  value = azurerm_linux_virtual_machine.cwagent.virtual_machine_id
}

output "cwagent_role_arn" {
  value = aws_iam_role.cwagent.arn
}

output "cwagent_principal_id" {
  value = azurerm_linux_virtual_machine.cwagent.identity[0].principal_id
}

output "testing_id" {
  value = module.common.testing_id
}
