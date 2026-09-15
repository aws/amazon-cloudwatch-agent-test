// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

output "cwagent_public_ip" {
  value = google_compute_instance.cwagent.network_interface[0].access_config[0].nat_ip
}

output "cwagent_instance_id" {
  value = google_compute_instance.cwagent.instance_id
}

output "cwagent_role_arn" {
  value = aws_iam_role.cwagent.arn
}

output "cwagent_sa_unique_id" {
  value = google_service_account.cwagent.unique_id
}

output "testing_id" {
  value = module.common.testing_id
}
