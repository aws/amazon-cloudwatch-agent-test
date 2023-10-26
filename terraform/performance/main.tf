// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../common"
}

module "basic_components" {
  source = "../basic_components"

  region = var.region
}

locals {
  ami_family        = var.ami_family[var.family]
  login_user        = local.ami_family["login_user"]
  install_package   = local.ami_family["install_package"]
  install_validator = local.ami_family["install_validator"]
  temp_directory    = local.ami_family["temp_folder"]
  connection_type   = local.ami_family["connection_type"]
  start_command     = format(local.ami_family["start_command"], module.validator.instance_agent_config)
}


#####################################################################
# Prepare Parameters Tests
#####################################################################

module "validator" {
  source = "../validator"

  arc                 = var.arc
  family              = var.family
  action              = "upload"
  s3_bucket           = var.s3_bucket
  test_dir            = var.test_dir
  temp_directory      = local.temp_directory
  cwa_github_sha      = var.cwa_github_sha
  cwa_github_sha_date = var.cwa_github_sha_date
  values_per_minute   = var.values_per_minute
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
  get_password_data                    = local.connection_type == "winrm" ? true : false
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-performance-${var.family}-${module.common.testing_id}"
  }
}

resource "null_resource" "install_binaries" {
  depends_on = [aws_instance.cwagent, module.validator]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
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
      local.ami_family["wait_cloud_init"],
      "aws s3 cp s3://${var.s3_bucket}/integration-test/packaging/${var.cwa_github_sha}/amazon-cloudwatch-agent.msi .",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/${var.family}/${var.arc}/${local.install_package} .",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/${var.family}/${var.arc}/${local.install_validator} .",
      local.ami_family["install_command"],
    ]
  }
}

resource "null_resource" "validator_linux" {
  count      = var.family != "windows" ? 1 : 0
  depends_on = [null_resource.install_binaries]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }
  provisioner "remote-exec" {
    inline = [
      #mock server dependencies getting transfered.
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      var.run_mock_server ? "cd mockserver && sudo docker build -t mockserver . && cd .." : "echo skipping mock server build",
      var.run_mock_server ? "sudo docker run --name mockserver -d -p 8080:8080 -p 443:443  mockserver" : "echo skipping mock server run",
      "pwd",
      "ls -lirt",
#      "cd ..", # return to root , two copy xray configs next to validator

      "pwd",
      "ls -lirt",
      "cp -r amazon-cloudwatch-agent-test/test/xray/resources /home/ec2-user/",
      "export AWS_REGION=${var.region}",
      "cd ./validator/validators",
      "ls -lirt",
      "pwd",
      "sudo chmod +x ./${local.install_validator}",
      "./${local.install_validator} --validator-config=${module.validator.instance_validator_config} --preparation-mode=true",
      local.start_command,
      "./${local.install_validator} --validator-config=${module.validator.instance_validator_config} --preparation-mode=false",
    ]
  }
}

resource "null_resource" "validator_windows" {
  count      = var.family == "windows" ? 1 : 0
  depends_on = [null_resource.install_binaries]

  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
    timeout     = "10m"
  }

  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "${local.install_validator} --validator-config=${module.validator.instance_validator_config}--preparation-mode=true",
      local.start_command,
      "${local.install_validator} --validator-config=${module.validator.instance_validator_config} --preparation-mode=false",
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

data "aws_dynamodb_table" "performance-dynamodb-table" {
  name = module.common.performance-dynamodb-table
}
