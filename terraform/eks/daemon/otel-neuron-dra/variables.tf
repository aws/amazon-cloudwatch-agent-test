// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/neuron_dra"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The chart must render the DRA correlation config (dra_device_types incl. the
# neuron-dra entry keyed on the neuron.aws.com driver) and grant the agent
# ServiceAccount resource.k8s.io RBAC. Override to a fork/branch until that lands
# on main (helm-charts PR #356).
variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "helm_chart_repo_url" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

# DRA (resource.k8s.io) is GA (v1) in k8s 1.34, which still serves v1beta1 — the
# version the processor's DRA informers watch. Keep at 1.34 until the client is
# bumped to v1.
variable "k8s_version" {
  type    = string
  default = "1.34"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_NEURON"
}

# Neuron (Trainium) node: trn1.2xlarge = 1 device × 2 cores.
# Must be a Trainium type: the Neuron DRA driver (driver image 1.2.0) supports
# Trainium only and rejects Inferentia (inf1/inf2) at device discovery. trn1.2xlarge
# is the smallest/most-available Trainium instance, enough to exercise DRA
# per-device correlation (1 claimed device -> both cores -> burn pod).
variable "instance_type" {
  type    = string
  default = "trn1.2xlarge"
}

# Neuron DRA driver Helm chart (OCI). draDriver.enabled=true installs the DRA
# driver (DeviceClass neuron.aws.com); devicePlugin must be disabled (the two
# cannot coexist on a node).
variable "neuron_helm_chart_version" {
  type    = string
  default = ""
}
