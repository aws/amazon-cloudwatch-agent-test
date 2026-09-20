// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

resource "random_password" "admin" {
  length      = 24
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
  min_special = 1
  # Azure rejects '"', '\'' and '\' in the Windows admin password; restrict special chars to a safe set.
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

locals {
  test_repo_base    = replace(var.github_test_repo, ".git", "")
  test_repo_zip_url = "${local.test_repo_base}/archive/refs/heads/${var.github_test_repo_branch}.zip"

  # Windows NetBIOS computer name is capped at 15 chars; testing_id alone is 16 hex chars, so derive a
  # short, unique name. The full Azure resource name below keeps the readable, un-truncated form.
  computer_name = substr("cwa${module.common.testing_id}", 0, 15)

  # Bootstrap PowerShell run once by the CustomScriptExtension (as SYSTEM, over the Azure fabric -- it
  # does not need WinRM to already work). Stands up a WinRM HTTPS (5986) listener with a self-signed
  # cert so the runner's provisioner can connect. HTTPS gives transport-level encryption, so
  # AllowUnencrypted stays false and Basic auth is disabled -- the runner authenticates with NTLM.
  winrm_bootstrap = <<-POWERSHELL
    $ErrorActionPreference = 'Stop'
    Set-Service -Name WinRM -StartupType Automatic
    Start-Service -Name WinRM
    # Local (non-builtin-Administrator) admin accounts otherwise get a UAC-filtered token over WinRM,
    # which would block the provisioner's writes to Program Files / ProgramData. Disable remote UAC
    # token filtering so the provisioning session runs with a full admin token.
    New-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Name 'LocalAccountTokenFilterPolicy' -Value 1 -PropertyType DWord -Force | Out-Null
    $cert = New-SelfSignedCertificate -DnsName $env:COMPUTERNAME -CertStoreLocation Cert:\LocalMachine\My
    $hasHttps = Get-ChildItem WSMan:\localhost\Listener | Where-Object { $_.Keys -contains 'Transport=HTTPS' }
    if (-not $hasHttps) {
      New-Item -Path WSMan:\localhost\Listener -Transport HTTPS -Address * -CertificateThumbPrint $cert.Thumbprint -Force
    }
    # Require encryption; NTLM/Negotiate only, never Basic-over-HTTP.
    Set-Item -Path WSMan:\localhost\Service\AllowUnencrypted -Value $false
    Set-Item -Path WSMan:\localhost\Service\Auth\Basic -Value $false
    Set-Item -Path WSMan:\localhost\Service\Auth\Negotiate -Value $true
    New-NetFirewallRule -DisplayName 'WinRM HTTPS 5986' -Name 'WINRM-HTTPS-In-TCP' -Profile Any -Direction Inbound -Action Allow -Protocol TCP -LocalPort 5986 | Out-Null
    Restart-Service -Name WinRM
  POWERSHELL

  # Install + run script, executed in a SINGLE PowerShell process. WinRM runs each remote-exec inline
  # command in an isolated shell, so env vars set in one command would not survive to the next; keeping
  # the whole flow in one process preserves PATH and the AWS_* web-identity env for `go test`.
  integration_test_script = <<-POWERSHELL
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    Write-Host "sha ${var.cwa_github_sha}"

    # Install the agent from the MSI. The Azure VM has no AWS instance profile, so unlike the EC2 Windows
    # suite it cannot 'aws s3 cp' the MSI itself; the workflow presigns the S3 object and the VM downloads
    # it here over HTTPS. This replaced a WinRM file transfer of the MSI, which was too slow (~41 min
    # observed) and blew the apply-step timeout. The presigned URL arrives in a tiny txt file
    # (file-provisioned above) so it stays out of this script's EncodedCommand, which the Actions log echoes.
    $msi = "C:\Users\${var.admin_username}\amazon-cloudwatch-agent.msi"
    $urlFile = "C:\Users\${var.admin_username}\agent_msi_url.txt"
    $url = (Get-Content -Path $urlFile -Raw).Trim()
    # The Windows Server 2022 image ships curl.exe; -fsSL fails on HTTP errors and follows redirects.
    curl.exe -fsSL --retry 3 -o $msi $url
    if (-not (Test-Path $msi) -or (Get-Item $msi).Length -eq 0) { throw "agent MSI download failed or empty: $msi" }
    Write-Host "downloaded MSI bytes: $((Get-Item $msi).Length)"
    Remove-Item -Force $urlFile -ErrorAction SilentlyContinue
    Start-Process msiexec.exe -ArgumentList '/i', $msi, '/norestart', '/qn' -Wait
    $ctl = 'C:\Program Files\Amazon\AmazonCloudWatchAgent\amazon-cloudwatch-agent-ctl.ps1'
    for ($i = 0; $i -lt 30 -and -not (Test-Path $ctl); $i++) { Start-Sleep -Seconds 5 }
    if (-not (Test-Path $ctl)) { throw "amazon-cloudwatch-agent-ctl.ps1 not found after install" }

    # Install Go matching the test repo go.mod directive, into C:\go, on PATH for this session only.
    $goZip = "$env:TEMP\go.zip"
    Invoke-WebRequest -Uri "https://go.dev/dl/go${var.go_version}.windows-amd64.zip" -OutFile $goZip
    Expand-Archive -Path $goZip -DestinationPath 'C:\' -Force
    $env:PATH = "C:\go\bin;$env:PATH"

    # Fetch the test repo WITHOUT git: the Windows Server image ships no git and installing it over
    # WinRM is flaky, so download the branch tarball and expand it (equivalent to the linux git clone).
    $repoZip = "$env:TEMP\cwatest.zip"
    Invoke-WebRequest -Uri "${local.test_repo_zip_url}" -OutFile $repoZip
    Expand-Archive -Path $repoZip -DestinationPath 'C:\cwatest' -Force
    # GitHub names the extracted top folder <repo>-<branch>; resolve it rather than hardcode.
    $testRepoDir = Get-ChildItem -Path 'C:\cwatest' -Directory | Select-Object -First 1 -ExpandProperty FullName
    Set-Location $testRepoDir

    # Persist env vars via ctl set-env (agent loads env-config.json at startup; OTel expandconverter
    # resolves ${var.region} / the role ARN in the translated YAML), then start on default:otel.
    # -m auto / -c default:otel exercise exactly the ctl.ps1 code paths that the stale-script bug broke.
    & $ctl -a set-env -e AWS_REGION=${var.region}
    if ($LASTEXITCODE -ne 0) { throw "amazon-cloudwatch-agent-ctl.ps1 -a set-env failed ($LASTEXITCODE)" }
    & $ctl -a set-env -e CWAGENT_ROLE_ARN=${module.iam.role_arn}
    if ($LASTEXITCODE -ne 0) { throw "amazon-cloudwatch-agent-ctl.ps1 -a set-env failed ($LASTEXITCODE)" }
    & $ctl -a fetch-config -m auto -s -c default:otel
    if ($LASTEXITCODE -ne 0) { throw "amazon-cloudwatch-agent-ctl.ps1 -a fetch-config failed ($LASTEXITCODE)" }

    # Mint the web-identity token from Azure IMDS (PowerShell equivalent of the linux curl). The token
    # is a bearer credential, so write it to a file whose ACL grants only this user, and remove it once
    # the test finishes.
    $tokenFile = "$env:TEMP\azure-identity-token"
    $resp = Invoke-RestMethod -Headers @{Metadata='true'} -Uri "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=${var.azure_token_audience}"
    Set-Content -Path $tokenFile -Value $resp.access_token -NoNewline
    icacls $tokenFile /inheritance:r | Out-Null
    icacls $tokenFile /grant:r "$($env:USERNAME):(M)" | Out-Null
    $env:AWS_WEB_IDENTITY_TOKEN_FILE = $tokenFile
    $env:AWS_ROLE_ARN = "${module.iam.role_arn}"
    $env:AWS_REGION = "${var.region}"

    go test -tags integration ${var.test_dir} -p 1 -timeout 30m -computeType=AZUREVM -instancePlatform=windows -region=${var.region} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${azurerm_windows_virtual_machine.cwagent.virtual_machine_id} -assumeRoleArn=${module.iam.role_arn} -v
    $rc = $LASTEXITCODE
    Remove-Item -Force $tokenFile -ErrorAction SilentlyContinue
    exit $rc
  POWERSHELL
}

# Azure Windows VM; its system-assigned managed identity is what oidctoken exchanges for an AWS session.
resource "azurerm_network_interface" "cwagent" {
  name                = "cwa-azurevmwin-integ-nic-${module.common.testing_id}"
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
  name                = "cwa-azurevmwin-integ-pip-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group
  allocation_method   = "Static"
}

# Allow inbound WinRM (HTTPS 5986) from the runner only (Azure's implicit default denies all inbound).
resource "azurerm_network_security_group" "cwagent" {
  name                = "cwa-azurevmwin-integ-nsg-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group

  security_rule {
    name                       = "AllowWinRMFromRunner"
    priority                   = 1000
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "5986"
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

resource "azurerm_windows_virtual_machine" "cwagent" {
  name                  = "cwa-azurevmwin-integ-${module.common.testing_id}"
  computer_name         = local.computer_name
  location              = var.azure_location
  resource_group_name   = var.azure_resource_group
  size                  = var.azure_vm_size
  admin_username        = var.admin_username
  admin_password        = random_password.admin.result
  network_interface_ids = [azurerm_network_interface.cwagent.id]

  # System-assigned managed identity: the token source for cross-cloud AssumeRoleWithWebIdentity.
  identity {
    type = "SystemAssigned"
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

# Configure WinRM at boot so no manual step is needed. Runs as SYSTEM via the Azure guest agent, so it
# is delivered over the Azure fabric (not WinRM) and can bootstrap WinRM itself. EncodedCommand takes a
# UTF-16LE base64 payload (textencodebase64), which sidesteps all cmd/PowerShell quoting of the script.
resource "azurerm_virtual_machine_extension" "winrm_setup" {
  name                       = "winrm-setup"
  virtual_machine_id         = azurerm_windows_virtual_machine.cwagent.id
  publisher                  = "Microsoft.Compute"
  type                       = "CustomScriptExtension"
  type_handler_version       = "1.10"
  auto_upgrade_minor_version = true

  settings = jsonencode({
    commandToExecute = "powershell.exe -ExecutionPolicy Unrestricted -EncodedCommand ${textencodebase64(local.winrm_bootstrap, "UTF-16LE")}"
  })
}

#####################################################################
# Install the agent, start it with default:otel, and run the test.
#####################################################################
resource "null_resource" "integration_test" {
  connection {
    type     = "winrm"
    user     = var.admin_username
    password = random_password.admin.result
    host     = azurerm_public_ip.cwagent.ip_address
    port     = 5986
    https    = true
    # Self-signed listener cert -> skip CA validation. use_ntlm keeps auth off Basic-over-HTTP while
    # HTTPS provides the transport encryption (AllowUnencrypted stays false on the service).
    insecure = true
    use_ntlm = true
    timeout  = "10m"
  }

  # Write only the presigned URL over WinRM (a tiny file transfers in seconds); the VM downloads the MSI
  # itself in the script above. Streaming the large MSI over the WinRM file provisioner was too slow
  # (~41 min observed) and blew the apply-step timeout. Keeping the URL in a file rather than in the
  # remote-exec command line also keeps it out of the public Actions log, which echoes the EncodedCommand.
  provisioner "file" {
    content     = var.agent_msi_url
    destination = "C:\\Users\\${var.admin_username}\\agent_msi_url.txt"
  }

  # Whole install+start+test flow in one PowerShell process (see local.integration_test_script).
  provisioner "remote-exec" {
    inline = [
      "powershell.exe -ExecutionPolicy Bypass -EncodedCommand ${textencodebase64(local.integration_test_script, "UTF-16LE")}",
    ]
  }

  depends_on = [
    azurerm_windows_virtual_machine.cwagent,
    azurerm_virtual_machine_extension.winrm_setup,
    azurerm_network_interface_security_group_association.cwagent,
    module.iam,
  ]
}
