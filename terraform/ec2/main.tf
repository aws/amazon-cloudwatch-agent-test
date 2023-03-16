// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../basic_components"

  region = var.region
}

locals {
  selected_ami             = var.family
  ami_family               = var.ami_family[local.selected_ami]
  connection_type          = local.ami_family["connection_type"]
  agent_config_destination = local.ami_family["agent_config_destination"]
  login_user               = lookup(local.selected_ami, "ec2-user", local.ami_family["login_user"])
  user_data                = lookup(local.selected_ami, "user_data", local.ami_family["user_data"])
  download_command         = format(local.ami_family["download_command_pattern"], "https://${var.package_s3_bucket}.s3.amazonaws.com/${local.selected_ami["os_family"]}/${local.selected_ami["arch"]}/${var.aoc_version}/${local.ami_family["install_package"]}")
  // Canary downloads latest binary. Integration test downloads binary connect to git hash.
  binary_uri = var.is_canary ? "${var.s3_bucket}/release/amazon_linux/${var.arc}/latest/${var.binary_name}" : "${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name}"
}

#####################################################################
# Prepare Parameters Tests
#####################################################################

locals {
  validator_config        = "parameters.yml"
  final_validator_config  = "final_parameters.yml"
  cloudwatch_agent_config = "agent_config.json"
}

resource "local_file" "update-validation-config" {
  content = replace(file("${var.test_dir}/${local.validator_config}"),
  "<cloudwatch_agent_config>", "${local.instance_temp_directory}/${local.cloudwatch_agent_config}")

  filename = "${var.test_dir}/${local.final_validator_config}"
}

// Build and uploading the validator to spending less time in 
// and avoid memory issue in allocating memory with Windows
resource "null_resource" "build-validator" {
  provisioner "local-exec" {
    command = "cd ../.. && make validator-build"
  }
}

resource "aws_s3_object" "upload-validator" {
  bucket     = var.s3_bucket
  key        = "integration-test/validator/${var.cwa_github_sha}/windows_${var.arc}/validator.exe"
  source     = "../../build/validator/windows_${var.arc}/validator.exe"
  depends_on = [null_resource.build-validator]
}

#####################################################################
# Create EFS
#####################################################################
resource "aws_efs_file_system" "efs" {
  count          = local.selected_ami == "" ? 1 : 0
  creation_token = "efs-${module.common.testing_id}"
  tags = {
    Name = "efs-${module.common.testing_id}"
  }
}

resource "aws_efs_mount_target" "mount" {
  count           = local.selected_ami == "" ? 1 : 0
  file_system_id  = aws_efs_file_system.efs.id
  subnet_id       = aws_instance.cwagent.subnet_id
  security_groups = [module.basic_components.security_group]
}

resource "null_resource" "mount_efs" {
  count = local.selected_ami == "" ? 1 : 0

  connection {
    type        = "ssh"
    user        = var.user
    private_key = local.private_key_content
    host        = aws_instance.cwagent.public_ip
  }

  provisioner "remote-exec" {
    # https://docs.aws.amazon.com/efs/latest/ug/mounting-fs-mount-helper-ec2-linux.html
    inline = [
      "sudo mkdir ~/efs-mount-point",
      "sudo mount -t efs -o tls ${aws_efs_file_system.efs.dns_name} ~/efs-mount-point/",
    ]
  }

  depends_on = [
    aws_efs_mount_target.mount,
    aws_instance.cwagent
  ]
}

#####################################################################
# Generate EC2 Instance and execute test commands
#####################################################################
resource "aws_instance" "cwagent" {
  ami                         = data.aws_ami.latest.id
  instance_type               = var.ec2_instance_type
  key_name                    = local.ssh_key_name
  iam_instance_profile        = module.basic_components.instance_profile
  subnet_id                   = module.basic_components.random_subnet_instance_id
  vpc_security_group_ids      = [module.basic_components.security_group]
  associate_public_ip_address = true
  get_password_data           = var.family == "windows" ? true : null

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-${var.test_name}-${module.common.testing_id}"
  }
}


resource "null_resource" "validator" {
  connection {
    type        = local.connection_type
    user        = local.login_user
    private_key = local.connection_type == "ssh" ? local.private_key_content : null
    password    = local.connection_type == "winrm" ? rsadecrypt(aws_instance.cwagent.password_data, local.private_key_content) : null
    host        = aws_instance.cwagent.public_dns
  }

  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      local.ami_family["wait_cloud_init"],
      local.download_command,
      local.ami_family["install_command"],
    ]
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
      local.ami_family["wait_cloud_init"],
      local.download_command,
      local.ami_family["install_command"],
    ]


  }

  provisioner "remote-exec" {
    inline = [
      "aws s3 cp s3://${var.s3_bucket}/integration-test/validator/${var.cwa_github_sha}/windows_${var.arc}/validator.exe .",
    ]
  }

  # Run validator 
  provisioner "remote-exec" {
    inline = [
      "set AWS_REGION=${var.region}",
      "validator.exe --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=true",
      "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c file:${local.instance_temp_directory}/${local.cloudwatch_agent_config}\"",
      "validator.exe --validator-config=${local.instance_temp_directory}/${local.final_validator_config} --preparation-mode=false",
    ]
  }
  depends_on = [
    aws_instance.cwagent,
    null_resource.mount_efs
  ]
}

data "aws_ami" "latest" {
  most_recent = true

  filter {
    name   = "name"
    values = [var.ami]
  }
}
