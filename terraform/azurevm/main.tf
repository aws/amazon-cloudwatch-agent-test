// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# SSH key for connecting to the Azure VM
#####################################################################
resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Azure Linux VM; its system-assigned managed identity is what oidctoken exchanges for an AWS session.
resource "azurerm_network_interface" "cwagent" {
  name                = "cwa-azurevm-integ-nic-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group

  ip_configuration {
    name                          = "internal"
    subnet_id                     = data.azurerm_subnet.selected.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.cwagent.id
  }
}

resource "azurerm_public_ip" "cwagent" {
  name                = "cwa-azurevm-integ-pip-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group
  allocation_method   = "Static"
}

# Allow inbound SSH from the runner only (Azure's implicit default denies all inbound).
resource "azurerm_network_security_group" "cwagent" {
  name                = "cwa-azurevm-integ-nsg-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group

  security_rule {
    name                       = "AllowSSHFromRunner"
    priority                   = 1000
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = var.runner_ip
    destination_address_prefix = "*"
  }
}

resource "azurerm_network_interface_security_group_association" "cwagent" {
  network_interface_id      = azurerm_network_interface.cwagent.id
  network_security_group_id = azurerm_network_security_group.cwagent.id
}

# Attach to an existing vnet/subnet in the resource group so CI needs no networking-create perms.
data "azurerm_virtual_network" "selected" {
  resource_group_name = var.azure_resource_group
  name                = var.azure_vnet_name
}

data "azurerm_subnet" "selected" {
  name                 = var.azure_subnet_name
  virtual_network_name = data.azurerm_virtual_network.selected.name
  resource_group_name  = var.azure_resource_group
}

resource "azurerm_linux_virtual_machine" "cwagent" {
  name                            = "cwa-azurevm-integ-${module.common.testing_id}"
  location                        = var.azure_location
  resource_group_name             = var.azure_resource_group
  size                            = var.azure_vm_size
  admin_username                  = var.admin_username
  disable_password_authentication = true
  network_interface_ids           = [azurerm_network_interface.cwagent.id]

  # System-assigned managed identity: the token source for cross-cloud AssumeRoleWithWebIdentity.
  identity {
    type = "SystemAssigned"
  }

  admin_ssh_key {
    username   = var.admin_username
    public_key = tls_private_key.ssh_key.public_key_openssh
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = var.azure_image.publisher
    offer     = var.azure_image.offer
    sku       = var.azure_image.sku
    version   = var.azure_image.version
  }
}

#####################################################################
# Install the agent, start it with default:otel, and run the test.
#####################################################################
resource "null_resource" "integration_test" {
  connection {
    type        = "ssh"
    user        = var.admin_username
    private_key = tls_private_key.ssh_key.private_key_pem
    host        = azurerm_public_ip.cwagent.ip_address
  }

  # Upload the runner-built .deb straight over the SSH connection (no S3 or public URL).
  provisioner "file" {
    source      = var.agent_deb_path
    destination = "/home/${var.admin_username}/amazon-cloudwatch-agent.deb"
  }

  # Install Go, clone the test repo, and install the uploaded agent .deb.
  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait",
      "echo sha ${var.cwa_github_sha}",
      "sudo apt-get update -y && sudo apt-get install -y golang-go git",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
      "sudo dpkg -i -E amazon-cloudwatch-agent.deb",
    ]
  }

  # Persist env vars to env-config.json: the agent loads this at startup, making them available to OTel
  # expandconverter which resolves ${AWS_REGION} and ${CWAGENT_ROLE_ARN} placeholders in the translated YAML.
  provisioner "remote-exec" {
    inline = [
      "export PATH=$PATH:/usr/local/go/bin",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent -setenv 'AWS_REGION=${var.region}' -envconfig /opt/aws/amazon-cloudwatch-agent/etc/env-config.json",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent -setenv 'CWAGENT_ROLE_ARN=${aws_iam_role.cwagent.arn}' -envconfig /opt/aws/amazon-cloudwatch-agent/etc/env-config.json",
      "sudo USE_DEFAULT_CONFIG=otel /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m auto -s -c default:otel",
      "cd amazon-cloudwatch-agent-test",
      "go test -tags integration ${var.test_dir} -p 1 -timeout 30m -computeType=AZUREVM -region=${var.region} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${azurerm_linux_virtual_machine.cwagent.virtual_machine_id} -assumeRoleArn=${aws_iam_role.cwagent.arn} -v",
    ]
  }

  depends_on = [
    azurerm_linux_virtual_machine.cwagent,
    azurerm_network_interface_security_group_association.cwagent,
    aws_iam_role_policy.cwagent,
  ]
}
