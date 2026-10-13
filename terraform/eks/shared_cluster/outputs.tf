// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Outputs an attaching test or workflow needs to build a kubeconfig for the
// shared cluster (for example: aws eks update-kubeconfig --name <cluster_name>),
// then deploy into its own per-run namespace.

output "cluster_name" {
  value = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  value = aws_eks_cluster.this.endpoint
}

output "cluster_ca_certificate" {
  value = aws_eks_cluster.this.certificate_authority[0].data
}

output "cluster_security_group_id" {
  value = aws_security_group.eks_cluster_sg.id
}
