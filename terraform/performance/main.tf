#####################################################################
# Ensure there is unique testing_id for each test
#####################################################################
resource "random_id" "testing_id" {
  byte_length = 8
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
  key_name   = "ec2-key-pair-${random_id.testing_id.hex}"
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
  iam_instance_profile        = aws_iam_instance_profile.cwagent_instance_profile.name
  vpc_security_group_ids      = [aws_security_group.ec2_security_group.id]
  associate_public_ip_address = true

  tags = {
    Name = "cwagent-integ-test-ec2-${var.test_name}-${random_id.testing_id.hex}"
  }
}

resource "null_resource" "integration_test" {
  # Prepare Integration Test
  provisioner "remote-exec" {
    inline = [
      "echo sha ${var.cwa_github_sha}",
      "cloud-init status --wait",
      "echo clone and install agent",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
      "cd amazon-cloudwatch-agent-test",
      "aws s3 cp s3://${var.s3_bucket}/integration-test/binary/${var.cwa_github_sha}/linux/${var.arc}/${var.binary_name} .",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      var.install_agent,
    ]

    connection {
      type        = "ssh"
      user        = var.user
      private_key = local.private_key_content
      host        = aws_instance.cwagent.public_ip
    }
  }

  #Run sanity check and integration test
  provisioner "remote-exec" {
    inline = [
      "echo prepare environment",
      "export AWS_REGION=${var.region}",
      "export PATH=$PATH:/snap/bin:/usr/local/go/bin",
      "echo run integration test",
      "cd ~/amazon-cloudwatch-agent-test",
      "export SHA=${var.cwa_github_sha}",
      "export SHA_DATE=${var.cwa_github_sha_date}",
      "export PERFORMANCE_NUMBER_OF_LOGS=${var.performance_number_of_logs}",
      "go test ${var.test_dir} -p 1 -timeout 1h -v --tags=integration "
    ]
    connection {
      type        = "ssh"
      user        = var.user
      private_key = local.private_key_content
      host        = aws_instance.cwagent.public_ip
    }
  }

  depends_on = [aws_instance.cwagent]
}

data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self", "506463145083"]

  filter {
    name   = "name"
    values = [var.ami]
  }
}