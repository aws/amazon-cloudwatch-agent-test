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

data "template_file" "windows-userdata" {
  template = <<EOF
<powershell>
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))
choco install git --confirm
choco install go --confirm
msiexec /i https://awscli.amazonaws.com/AWSCLIV2.msi  /norestart /qb-
[Environment]::SetEnvironmentVariable("PATH", "C:\ProgramData\chocolatey\bin;C:\Program Files\Git\cmd;C:\Program Files\Amazon\AWSCLIV2\;C:\Program Files\Go\bin;C:\Windows\System32;C:\Windows\System32\WindowsPowerShell\v1.0\", [System.EnvironmentVariableTarget]::Machine)
</powershell>
EOF
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
  get_password_data           = true
  user_data                   = data.template_file.windows-userdata.rendered 

  tags = {
    Name = "cwagent-integ-test-ec2-windows-${module.common.testing_id}"
  }
}

resource "null_resource" "integration_test" {
  depends_on = [aws_instance.cwagent]
  
  connection {
      type            = "winrm"
      user            = "Administrator"
      password        = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
      host            = aws_instance.cwagent.public_dns
      target_platform = "windows"
    }

  # Install software
  provisioner "remote-exec" {
    inline = [                                               
      "call %ProgramData%\\chocolatey\\bin\\RefreshEnv.cmd", //Reload the environment variables to pull the latest one instead of restarting cmd
      "set AWS_REGION=${var.region}",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "start /wait msiexec /i amazon-cloudwatch-agent.msi /norestart /qb-",
      "echo clone and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "echo run tests with the tag integration, one at a time, and verbose",
      "echo run sanity test && go test ./test/sanity -p 1 -v",
      "go test ${var.test_dir} -p 1 -timeout 30m -v"
    ]
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
