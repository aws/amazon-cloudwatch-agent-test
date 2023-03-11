// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
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
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
  instance_temp_directory = "C:\\"
}

resource "local_file" "update-validation-config" {
  content = replace(file("${var.test_dir}/${local.validator_config}"),
  "<cloudwatch_agent_config>", "${local.instance_temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}
#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################

resource "aws_instance" "cwagent" {
  ami                         = data.aws_ami.latest.id
  instance_type               = var.ec2_instance_type
  key_name                    = local.ssh_key_name
  iam_instance_profile        = data.aws_iam_instance_profile.cwagent_instance_profile.name
  vpc_security_group_ids      = [data.aws_security_group.ec2_security_group.id]
  associate_public_ip_address = true
  user_data                   = <<EOF
<powershell>
# Install SSH
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0

# OPTIONAL but recommended:
Set-Service -Name sshd -StartupType 'Automatic'

# Start the sshd service
Start-Service sshd

# Confirm the Firewall rule is configured. It should be created automatically by setup. Run the following to verify
if (!(Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue | Select-Object Name, Enabled)) {
    Write-Output "Firewall Rule 'OpenSSH-Server-In-TCP' does not exist, creating it..."
    New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
} else {
    Write-Output "Firewall rule 'OpenSSH-Server-In-TCP' has been created and exists."
}

# Set default shell for OpenSSH
New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell -Value "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" -PropertyType String -Force;

# Get the public key 
$authorized_key = "${trimspace(tls_private_key.ssh_key[0].public_key_openssh)}"
New-Item -Force -ItemType Directory -Path $env:USERPROFILE\.ssh; Add-Content -Force -Path $env:USERPROFILE\.ssh\authorized_keys -Value $authorized_key

# Set the config to allow the pubkey auth
$sshd_config="C:\ProgramData\ssh\sshd_config" 
(Get-Content $sshd_config) -replace '#PubkeyAuthentication', 'PubkeyAuthentication' | Out-File -encoding ASCII $sshd_config
(Get-Content $sshd_config) -replace 'AuthorizedKeysFile __PROGRAMDATA__', '#AuthorizedKeysFile __PROGRAMDATA__' | Out-File -encoding ASCII $sshd_config
(Get-Content $sshd_config) -replace 'Match Group administrators', '#Match Group administrators' | Out-File -encoding ASCII $sshd_config
Get-Content C:\ProgramData\ssh\sshd_config

# Reload the config
Restart-Service sshd
</powershell>
EOF

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-windows-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent]
  connection {
    type            = "ssh"
    user            = "Administrator"
    private_key     = local.private_key_content
    host            = aws_instance.cwagent.public_ip
    target_platform = "windows"
    timeout         = "15m"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.final_validator_config}"
    destination = "${local.instance_temp_directory}/${local.final_validator_config}"
  }

  provisioner "file" {
    source      = "${var.test_dir}/${local.cloudwatch_agent_config}"
    destination = "${local.instance_temp_directory}/${local.cloudwatch_agent_config}"
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "sudo installer -pkg ./amazon-cloudwatch-agent.pkg -target /",
    ]
  }

  #Prepare the requirement before validation and validate the metrics/logs/traces
  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd ~/amazon-cloudwatch-agent-test",
      "go run ./validator/main.go --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=true",
      "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c file:${local.instance_temp_directory}/${local.cloudwatch_agent_config}",
      "go run ./validator/main.go --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}
