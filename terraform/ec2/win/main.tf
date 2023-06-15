// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
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
  ssm_parameter_name  = "WindowsAgentConfigSSMTest"
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

module "validator" {
  source = "../../validator"

  arc            = var.arc
  family         = "windows"
  action         = "upload"
  s3_bucket      = var.s3_bucket
  test_dir       = var.test_dir
  temp_directory = "C:/Users/Administrator/AppData/Local/Temp"
  cwa_github_sha = var.cwa_github_sha
}

#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################

resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
  vpc_security_group_ids               = [module.basic_components.security_group]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"
  get_password_data                    = true
  user_data = <<EOF
<powershell>
[Environment]::SetEnvironmentVariable("PATH", "C:\ProgramData\chocolatey\bin;C:\Program Files\Git\cmd;C:\Program Files\Amazon\AWSCLIV2\;C:\Program Files\Go\bin;C:\Windows\System32;C:\Windows\System32\WindowsPowerShell\v1.0\", [System.EnvironmentVariableTarget]::Machine)
</powershell>
EOF

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-windows-${var.test_name}-${module.common.testing_id}"
  }
}

# Size of windows json is too large thus can't use standard tier
resource "aws_ssm_parameter" "upload_ssm" {
  count = var.use_ssm == true && fileexists(module.validator.agent_config) == true ? 1 : 0
  name  = local.ssm_parameter_name
  type  = "String"
  tier  = "Advanced"
  value = file(module.validator.agent_config)
}

resource "null_resource" "integration_test_setup" {
  depends_on = [aws_instance.cwagent, module.validator, aws_ssm_parameter.upload_ssm]

  # Install software
  connection {
    type            = "ssh"
    user            = "Administrator"
    password        = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host            = aws_instance.cwagent.public_ip
    target_platform = "windows"
    timeout         = "6m"
    agent = false
    script_path = "c:/windows/temp/terraform_%RAND%.ps1"
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "start /wait timeout 120", //Wait some time to ensure all binaries have been downloaded
      "call %ProgramData%\\chocolatey\\bin\\RefreshEnv.cmd", //Reload the environment variables to pull the latest one instead of restarting cmd
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "start /wait msiexec /i amazon-cloudwatch-agent.msi /norestart /qb-",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/windows/${var.arc}/validator.exe .",
    ]
  }
}

## reboot when only needed
resource "null_resource" "integration_test_reboot" {
  count = length(regexall("restart", var.test_dir)) > 0 ? 1 : 0

  connection {
    type            = "ssh"
    user            = "Administrator"
    password        = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host            = aws_instance.cwagent.public_ip
    target_platform = "windows"
    timeout         = "6m"
    agent = false
    script_path = "c:/windows/temp/terraform_%RAND%.ps1"
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      "powershell \"Restart-Computer -Force\"",
    ]
  }

  depends_on = [
    null_resource.integration_test_setup,
  ]
}

resource "null_resource" "integration_test_wait" {
  count = length(regexall("restart", var.test_dir)) > 0 ? 1 : 0
  provisioner "local-exec" {
    command = <<-EOT
      echo "Sleeping after initiating instance restart"
      sleep 180
    EOT
  }
  depends_on = [
    null_resource.integration_test_reboot,
  ]
}

resource "null_resource" "integration_test_run" {
  # run go test when it's not feature test
  count = length(regexall("/feature/windows", var.test_dir)) < 1 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup,
    null_resource.integration_test_wait,
  ]

  connection {
    type            = "ssh"
    user            = "Administrator"
    password        = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host            = aws_instance.cwagent.public_ip
    target_platform = "windows"
    timeout         = "6m"
    agent = false
    script_path = "c:/windows/temp/terraform_%RAND%.ps1"
  }

  provisioner "remote-exec" {
    inline = [
#      "validator.exe --test-name=${var.test_dir}",
      "start /wait timeout 120", //Wait some time to ensure all binaries have been downloaded
      "call %ProgramData%\\chocolatey\\bin\\RefreshEnv.cmd", //Reload the environment variables to pull the latest one instead of restarting cmd
      "start /wait msiexec /i amazon-cloudwatch-agent.msi /norestart /qb-",
      "echo clone and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "echo running tests",
      "go test ${var.test_dir} -p 1 -timeout 30m -v "
    ]
  }
}

resource "null_resource" "integration_test_run_validator" {
  # run validator only when test_dir is not passed e.g. the default from variable.tf
  count = length(regexall("/feature/windows", var.test_dir)) > 0 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup,
    null_resource.integration_test_reboot,
  ]

  connection {
    type            = "ssh"
    user            = "Administrator"
    password        = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host            = aws_instance.cwagent.public_ip
    target_platform = "windows"
    timeout         = "6m"
    agent = false
    script_path = "c:/windows/temp/terraform_%RAND%.ps1"
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      var.use_ssm ? "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c ssm:${local.ssm_parameter_name}\"" : "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c file:${module.validator.instance_agent_config}\"",
      "validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=false",
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
