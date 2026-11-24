// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/otlp"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The chart is cloned from helm_chart_repo at helm_chart_branch, so a branch on
# a fork can be tested before it is merged.
variable "helm_chart_repo" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

# Installs the chart with otlp.profiles.enabled=true, makes the app sender post
# profiles and turns on TestProfiles (OTLP_PROFILES_ENABLED). The profiles
# pipeline needs an agent image with the collector's profiles support
# (1.300075.0 or later). The suite runs the image under test
# (cwagent_image_repo/cwagent_image_tag), not the chart's default image, so this
# is on by default; set it to false to test an older agent image.
variable "otlp_profiles_enabled" {
  type    = bool
  default = true
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

variable "instance_type" {
  type    = string
  default = "t3.medium"
}
