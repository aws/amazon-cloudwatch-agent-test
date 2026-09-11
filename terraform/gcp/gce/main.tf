// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# SSH key for connecting to the GCE VM
#####################################################################
resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

locals {
  # Subnetworks are regional; derive the region from the zone input (us-east1-b -> us-east1).
  gcp_region = join("-", slice(split("-", var.gcp_zone), 0, 2))
}

# Attach to an existing network/subnetwork in the project so CI needs no networking-create perms.
data "google_compute_network" "selected" {
  name = var.gcp_network_name
}

data "google_compute_subnetwork" "selected" {
  name   = var.gcp_subnetwork_name
  region = local.gcp_region
}

# Allow inbound SSH from the runner only, scoped to this VM via its target tag. Explicit so the
# module does not depend on the network's pre-existing firewall rules.
resource "google_compute_firewall" "cwagent" {
  name    = "cwa-gce-integ-fw-${module.common.testing_id}"
  network = data.google_compute_network.selected.self_link

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = [var.runner_ip]
  target_tags   = ["cwa-gce-integ-${module.common.testing_id}"]
}

# GCE Linux VM; its attached service account is what oidctoken exchanges for an AWS session.
resource "google_compute_instance" "cwagent" {
  name         = "cwa-gce-integ-${module.common.testing_id}"
  machine_type = var.gcp_machine_type
  zone         = var.gcp_zone
  tags         = ["cwa-gce-integ-${module.common.testing_id}"]

  boot_disk {
    initialize_params {
      image = var.gcp_image
    }
  }

  network_interface {
    subnetwork = data.google_compute_subnetwork.selected.self_link

    # Ephemeral public IP for the runner's SSH connection.
    access_config {}
  }

  # Attached per-run service account: the token source for cross-cloud AssumeRoleWithWebIdentity.
  # cloud-platform is the standard access scope; the account holds no IAM roles, so it grants nothing.
  service_account {
    email  = google_service_account.cwagent.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    ssh-keys = "${var.admin_username}:${tls_private_key.ssh_key.public_key_openssh}"
  }
}

#####################################################################
# Install the agent, start it with default:otel, and run the test.
#####################################################################
resource "null_resource" "integration_test" {
  connection {
    type        = "ssh"
    user        = var.admin_username
    private_key = tls_private_key.ssh_key.private_key_pem
    host        = google_compute_instance.cwagent.network_interface[0].access_config[0].nat_ip
  }

  # Upload the runner-built .deb straight over the SSH connection (no S3 or public URL).
  provisioner "file" {
    source      = var.agent_deb_path
    destination = "/home/${var.admin_username}/amazon-cloudwatch-agent.deb"
  }

  # Install Go, clone the test repo, and install the uploaded agent .deb.
  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait",
      "echo sha ${var.cwa_github_sha}",
      "sudo apt-get update -y && sudo apt-get install -y golang-go git",
      "git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo} -q",
      "sudo dpkg -i -E amazon-cloudwatch-agent.deb",
    ]
  }

  # Persist env vars with the ctl set-env action: the agent loads env-config.json at startup, making them
  # available to OTel expandconverter which resolves ${AWS_REGION} and ${CWAGENT_ROLE_ARN} in the
  # translated YAML. set-env runs before fetch-config so both are set on the first agent start.
  provisioner "remote-exec" {
    inline = [
      "export PATH=$PATH:/usr/local/go/bin",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a set-env -e 'AWS_REGION=${var.region}'",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a set-env -e 'CWAGENT_ROLE_ARN=${aws_iam_role.cwagent.arn}'",
      "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m auto -s -c default:otel",
      # The test binary validates delivery via AWS reads; give it the same web-identity chain the agent
      # uses. The token is a bearer credential, so create it 0600 (umask before redirect, since the
      # shell creates the file) and remove it once the test finishes.
      "umask 077 && curl -s -H 'Metadata-Flavor: Google' 'http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/identity?audience=${var.gcp_token_audience}' > /tmp/gcp-identity-token",
      "export AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/gcp-identity-token AWS_ROLE_ARN=${aws_iam_role.cwagent.arn} AWS_REGION=${var.region}",
      "cd amazon-cloudwatch-agent-test",
      "go test -tags integration ${var.test_dir} -p 1 -timeout 30m -computeType=GCE -region=${var.region} -cwaCommitSha=${var.cwa_github_sha} -instanceId=${google_compute_instance.cwagent.instance_id} -assumeRoleArn=${aws_iam_role.cwagent.arn} -v; test_rc=$?; rm -f /tmp/gcp-identity-token; exit $test_rc",
    ]
  }

  depends_on = [
    google_compute_instance.cwagent,
    google_compute_firewall.cwagent,
    aws_iam_role_policy.cwagent,
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
}
