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
Write-Output "Install OpenSSH and Firewalls which allows port 22 for connection"
Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0


Set-Service -Name sshd -StartupType 'Automatic'
Start-Service sshd

# Confirm the Firewall rule is configured. It should be created automatically by setup. Run the following to verify
if (!(Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue | Select-Object Name, Enabled)) {
    Write-Output "Firewall Rule 'OpenSSH-Server-In-TCP' does not exist, creating it..."
    New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
} else {
    Write-Output "Firewall rule 'OpenSSH-Server-In-TCP' has been created and exists."
}

# Set default shell for OpenSSH
New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell

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

[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

choco install git --confirm
choco install go --confirm
msiexec /i https://awscli.amazonaws.com/AWSCLIV2.msi  /norestart /qb-

[Environment]::SetEnvironmentVariable("PATH", "C:\ProgramData\chocolatey\bin;C:\Program Files\Git\cmd;C:\Program Files\Amazon\AWSCLIV2\;C:\Program Files\Go\bin;C:\Windows\System32;C:\Windows\System32\WindowsPowerShell\v1.0\", [System.EnvironmentVariableTarget]::Machine)
</powershell>
EOF

  tags = {
    Name = "cwagent-integ-test-ec2-windows-${element(split("/", var.test_dir), 3)}-$${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent]
  # Install software
  provisioner "remote-exec" {
    inline = [                                               //Wait some time to ensure all binaries have been downloaded
      "call %ProgramData%\\chocolatey\\bin\\RefreshEnv.cmd", //Reload the environment variables to pull the latest one instead of restarting cmd
      "set AWS_REGION=${var.region}",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "start /wait msiexec /i amazon-cloudwatch-agent.msi /norestart /qb-",
      "echo clone and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "git reset --hard ${var.cwa_test_github_sha}",
      "echo run tests with the tag integration, one at a time, and verbose",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      "go test ${var.test_dir} -p 1 -timeout 30m -v"
    ]

    connection {
      type            = "ssh"
      user            = "Administrator"
      private_key     = local.private_key_content
      host            = aws_instance.cwagent.public_dns
      target_platform = "windows"
      timeout         = "6m"
    }
  }
}

data "aws_ami" "latest" {
  most_recent = true
  // @Todo: Add back when nvidia_gpu pipeline has been able to produced the AMI
  #owners      = ["self", "506463145083"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}
