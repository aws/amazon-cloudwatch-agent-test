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
  ssm_parameter_name  = "WindowsAgentConfigSSMTest-${module.common.testing_id}"
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

module "validator" {
  source = "../../validator"

  arc       = var.arc
  family    = "windows"
  action    = "upload"
  s3_bucket = var.s3_bucket
  # hacky but gpu test dir is shared with linux which follows the pattern of ./*
  test_dir       = length(regexall("nvidia_gpu", var.test_dir)) > 0 ? "../../.${var.test_dir}" : var.test_dir
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
  user_data                            = length(regexall("/feature/windows/custom_start/userdata", var.test_dir)) > 0 ? data.template_file.user_data.rendered : ""
  get_password_data                    = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-windows-${var.test_name}-${module.common.testing_id}"
  }
  depends_on = [aws_ssm_parameter.upload_ssm]
}

# Size of windows json is too large thus can't use standard tier
resource "aws_ssm_parameter" "upload_ssm" {
  count = length(regexall("/feature/windows", var.test_dir)) > 0 ? 1 : 0
  name  = local.ssm_parameter_name
  type  = "String"
  tier  = "Advanced"
  value = file(module.validator.agent_config)
}

resource "null_resource" "integration_test_setup_agent" {
  count      = length(regexall("/feature/windows/custom_start/userdata", var.test_dir)) > 0 ? 0 : 1
  depends_on = [aws_instance.cwagent, module.validator, aws_ssm_parameter.upload_ssm]

  # Install software
  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "powershell.exe -Command \"Write-Output 'Checking for running msiexec processes...'; while (Get-Process msiexec -ErrorAction SilentlyContinue) { Write-Output 'msiexec running; sleeping 10s...'; Start-Sleep -Seconds 10 }; Write-Output 'No msiexec found; starting installer'; Start-Process -FilePath msiexec -ArgumentList '/i amazon-cloudwatch-agent.msi /norestart /qb-' -Wait\"",
      "if %errorlevel% neq 0 (echo MSI install failed with code %errorlevel% & exit 1)",
      "powershell.exe -Command \"$retries = 0; $maxRetries = 30; $found = $false; while ($retries -lt $maxRetries -and -not $found) { if (Test-Path 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1') { $found = $true; Write-Output 'amazon-cloudwatch-agent-ctl.ps1 found in expected location'; } else { $scriptPath = Get-ChildItem -Path 'C:\\Program Files' -Filter amazon-cloudwatch-agent-ctl.ps1 -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty FullName; if ($scriptPath) { $found = $true; Write-Output 'amazon-cloudwatch-agent-ctl.ps1 found at: ' + $scriptPath; } } if (-not $found) { Start-Sleep -Seconds 10; $retries++ } } if (-not $found) { Write-Output 'amazon-cloudwatch-agent-ctl.ps1 not found after installation and retries'; exit 1 }\""
    ]
  }
}
resource "null_resource" "integration_test_setup_validator" {
  depends_on = [aws_instance.cwagent, module.validator, aws_ssm_parameter.upload_ssm]

  # Install software
  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  # Install agent binaries
  provisioner "remote-exec" {
    inline = [
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/windows/${var.arc}/validator.exe C:\\validator.exe",
    ]
  }
}

## reboot when only needed
resource "null_resource" "integration_test_reboot" {
  count = length(regexall("restart", var.test_dir)) > 0 ? 1 : 0

  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      "powershell \"Restart-Computer -Force\"",
    ]
  }

  depends_on = [
    null_resource.integration_test_setup_agent,
    null_resource.integration_test_setup_validator,
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
    null_resource.integration_test_setup_agent,
    null_resource.integration_test_setup_agent,
    null_resource.integration_test_wait,
  ]

  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "cd C:\\Users\\Administrator\\amazon-cloudwatch-agent-test",
      "C:\\validator.exe --test-name=${var.test_dir} --computeType=EC2 --instanceId=${aws_instance.cwagent.id} --bucket=${var.s3_bucket}"
    ]
  }
}


resource "null_resource" "integration_test_run_validator" {
  # run validator only when test_dir is not passed e.g. the default from variable.tf
  count = length(regexall("/feature/windows", var.test_dir)) > 0 && length(regexall("/feature/windows/custom_start", var.test_dir)) < 1 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup_agent,
    null_resource.integration_test_setup_validator,
    null_resource.integration_test_wait,
  ]

  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  provisioner "file" {
    source      = "${path.module}/setup-jmx-workload.ps1"
    destination = "C:\\setup-jmx-workload.ps1"
  }

  //runs validator and sets up prometheus java agent
  provisioner "remote-exec" {
    inline = [
      # Setup JMX workload using PowerShell script
      "powershell.exe -ExecutionPolicy Bypass -File C:\\setup-jmx-workload.ps1",
      "set AWS_REGION=${var.region}",
      "cd C:\\Users\\Administrator",
      "echo Current directory before git clone:",
      "cd",
      "if exist amazon-cloudwatch-agent-test rmdir /s /q amazon-cloudwatch-agent-test",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "echo Current directory after cd:",
      "cd",
      "echo Listing test resources:",
      "dir test\\app_signals\\resources\\traces",
      "dir test\\app_signals\\resources\\metrics",
      "go test ./test/sanity -p 1 -v",
      "cd C:\\Users\\Administrator",

      # Retry logic for amazon-cloudwatch-agent-ctl.ps1 with timeout
      "powershell.exe -Command \"$maxRetries = 50; $retryCount = 0; while (-not (Test-Path 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1') -and $retryCount -lt $maxRetries) { Start-Sleep -s 10; $retryCount++ }; if ($retryCount -eq $maxRetries) { throw 'Timeout: amazon-cloudwatch-agent-ctl.ps1 not found' }\"",

      var.use_ssm ? "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c ssm:${local.ssm_parameter_name}\"" : "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c file:${module.validator.instance_agent_config}\"",
      "cd C:\\Users\\Administrator\\amazon-cloudwatch-agent-test",
      "echo Current directory before validator:",
      "cd",
      "C:\\validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=false"
    ]
  }
}

resource "null_resource" "integration_test_run_validator_start_agent_ssm" {
  # run validator only when test_dir is not passed e.g. the default from variable.tf
  count = length(regexall("/feature/windows/custom_start/ssm_start", var.test_dir)) > 0 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup_validator,
    null_resource.integration_test_wait,
  ]

  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  //runs validator and sets up prometheus java agent
  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "aws ssm send-command --document-name AmazonCloudWatch-ManageAgent --parameters optionalConfigurationLocation=${local.ssm_parameter_name} --targets Key=tag:Name,Values=cwagent-integ-test-ec2-windows-${var.test_name}-${module.common.testing_id}",
    ]
  }
}

resource "null_resource" "integration_test_run_validator_custom_start" {
  # run validator only when test_dir is not passed e.g. the default from variable.tf
  count = length(regexall("/feature/windows/custom_start", var.test_dir)) > 0 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup_validator,
    null_resource.integration_test_wait,
    null_resource.integration_test_run_validator_start_agent_ssm
  ]

  connection {
    type     = "winrm"
    user     = "Administrator"
    password = rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content)
    host     = aws_instance.cwagent.public_dns
  }

  provisioner "file" {
    source      = module.validator.agent_config
    destination = module.validator.instance_agent_config
  }

  provisioner "file" {
    source      = module.validator.validator_config
    destination = module.validator.instance_validator_config
  }

  //runs validator and sets up prometheus java agent
  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "cd C:\\Users\\Administrator\\amazon-cloudwatch-agent-test",
      "C:\\validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      "powershell.exe \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -m ec2 -a status\"",
      "C:\\validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=false"
    ]
  }
}

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "amazon"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}

#####################################################################
# Generate template file for EC2 userdata script
#####################################################################
data "template_file" "user_data" {
  template = file("install_and_start_agent.tpl")

  vars = {
    copy_object       = "Copy-S3Object -BucketName ${var.s3_bucket} -Key integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi -region ${var.region} -LocalFile $cwAgentInstaller"
    agent_json_config = local.ssm_parameter_name
  }
}

output "userdata" {
  value = data.template_file.user_data.rendered
}
