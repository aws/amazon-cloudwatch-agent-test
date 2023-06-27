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
  get_password_data                    = true

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
  count = var.use_ssm == true && length(regexall("/feature/windows", var.test_dir)) > 0 ? 1 : 0
  name  = local.ssm_parameter_name
  type  = "String"
  tier  = "Advanced"
  value = file(module.validator.agent_config)
}

resource "null_resource" "integration_test_setup" {
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
      "start /wait msiexec /i amazon-cloudwatch-agent.msi /norestart /qb-",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/windows/${var.arc}/validator.exe .",
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
      "validator.exe --test-name=${var.test_dir}"
    ]
  }
}

resource "null_resource" "integration_test_run_validator" {
  # run validator only when test_dir is not passed e.g. the default from variable.tf
  count = length(regexall("/feature/windows", var.test_dir)) > 0 ? 1 : 0
  depends_on = [
    null_resource.integration_test_setup,
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
      "mkdir C:\\jmx_workload",
      "powershell.exe -Command \"'---', 'rules:', '- pattern: \\\".*\\\"' | Set-Content -Path \\\"C:\\jmx_workload\\exporter_config.yaml\\\"\"",
      "powershell.exe -Command \"'global:', '  scrape_interval: 1m', '  scrape_timeout: 10s', 'scrape_configs:', '  - job_name: jmx-exporter', '    sample_limit: 10000', '    file_sd_configs:', '      - files: [ \\\"C:\\\\jmx_workload\\\\prometheus_file_sd.yaml\\\" ]' | Set-Content -Path \\\"C:\\jmx_workload\\prometheus.yaml\\\"\"",
      "powershell.exe -Command \"$env:AWS_IMDSV2_TOKEN = (Invoke-RestMethod -Uri 'http://169.254.169.254/latest/api/token' -Method 'PUT' -Headers @{ 'X-aws-ec2-metadata-token-ttl-seconds' = '300' }).trim(); $InstanceId = Invoke-RestMethod -Uri 'http://169.254.169.254/latest/meta-data/instance-id' -Headers @{ 'X-aws-ec2-metadata-token' = $env:AWS_IMDSV2_TOKEN };; Add-Content -Path 'C:\\jmx_workload\\prometheus_file_sd.yaml' '- targets:'; Add-Content -Path 'C:\\jmx_workload\\prometheus_file_sd.yaml' '  - 127.0.0.1:9404'; Add-Content -Path 'C:\\jmx_workload\\prometheus_file_sd.yaml' '  labels:'; Add-Content -Path 'C:\\jmx_workload\\prometheus_file_sd.yaml' '    application: test-app'; Add-Content -Path 'C:\\jmx_workload\\prometheus_file_sd.yaml' ('    InstanceId: ' + $InstanceId)\"",
      "powershell.exe -Command \"[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12\"",
      "powershell.exe -Command \"(New-Object Net.WebClient).DownloadFile('https://cwagent-prometheus-test.s3-us-west-2.amazonaws.com/jmx_prometheus_javaagent-0.12.0.jar', 'C:\\\\jmx_workload\\\\jmx_prometheus_javaagent-0.12.0.jar')\"",
      "powershell.exe -Command \"(New-Object Net.WebClient).DownloadFile('https://cwagent-prometheus-test.s3-us-west-2.amazonaws.com/SampleJavaApplication-1.0-SNAPSHOT.jar', 'C:\\\\jmx_workload\\\\SampleJavaApplication-1.0-SNAPSHOT.jar')\"",
      "powershell.exe -Command \"Start-Sleep -s 60\"",
      "powershell.exe -Command \"Start-Process -FilePath \\\"C:\\Program Files\\OpenJDK\\jdk-15.0.2\\bin\\java.exe\\\" -ArgumentList \\\"-javaagent:C:\\jmx_workload\\jmx_prometheus_javaagent-0.12.0.jar=9404:C:\\jmx_workload\\exporter_config.yaml -cp C:\\jmx_workload\\SampleJavaApplication-1.0-SNAPSHOT.jar com.gubupt.sample.app.App\\\"\"",
      "powershell.exe -Command \"Start-Sleep -s 60\"",
      "powershell.exe -Command \"Invoke-WebRequest -Uri http://localhost:9404 -UseBasicParsing\"",
      "set AWS_REGION=${var.region}",
      "validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      var.use_ssm ? "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c ssm:${local.ssm_parameter_name}\"" : "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c file:${module.validator.instance_agent_config}\"",
      "validator.exe --validator-config=${module.validator.instance_validator_config} --preparation-mode=false"
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
