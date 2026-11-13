// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/java_profiling"
}

# <registry or domain>/<repository>: the chart takes the two parts separately.
variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
  validation {
    condition     = can(regex("^[A-Za-z0-9.:-]+(/[A-Za-z0-9._-]+)+$", var.cwagent_image_repo))
    error_message = "cwagent_image_repo must be <registry>/<repository>."
  }
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The operator image. Empty keeps the chart's pinned operator; set both to test
# an operator build that the chart does not pin yet.
variable "cwagent_operator_image_repo" {
  type    = string
  default = ""
  validation {
    condition     = var.cwagent_operator_image_repo == "" || can(regex("^[A-Za-z0-9.:-]+(/[A-Za-z0-9._-]+)+$", var.cwagent_operator_image_repo))
    error_message = "cwagent_operator_image_repo must be empty or <registry>/<repository>."
  }
}

variable "cwagent_operator_image_tag" {
  type    = string
  default = ""
}

# The chart is cloned from helm_chart_repo at helm_chart_branch, so a branch on
# a fork can be tested before it is merged.
variable "helm_chart_repo" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
  validation {
    condition     = can(regex("^https://[A-Za-z0-9./_-]+$", var.helm_chart_repo))
    error_message = "helm_chart_repo must be an https:// git URL."
  }
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9._/-]*$", var.helm_chart_branch))
    error_message = "helm_chart_branch must be a branch name."
  }
}

# What the test expects of allocation profiling (JAVA_PROFILING_EXPECT_MEMORY):
# "true", the operator injects OTEL_AWS_PROFILER_MEMORY_ENABLED=true and the
# allocation profile types reach CloudWatch; "false", the profiled workloads set
# OTEL_AWS_PROFILER_MEMORY_ENABLED=false themselves, the operator keeps it and
# none reach CloudWatch.
variable "expect_memory_profiling" {
  type    = string
  default = "true"
  validation {
    condition     = contains(["true", "false"], var.expect_memory_profiling)
    error_message = "expect_memory_profiling must be \"true\" or \"false\"."
  }
}

# The chart's otlp.http.port. Not the default 4318, so that the test shows the
# profiler endpoint follows the chart's port; not 4317 (otlp.grpc.port) or a port
# the agent already uses, which the chart rejects.
variable "otlp_http_port" {
  type    = number
  default = 4328
  validation {
    condition = (
      var.otlp_http_port == floor(var.otlp_http_port) && var.otlp_http_port >= 1 && var.otlp_http_port <= 65535 &&
      !contains([4317, 4318, 2000, 4311, 4314, 4315, 4316, 8125, 25826, 25888], var.otlp_http_port)
    )
    error_message = "otlp_http_port must be a port other than 4318 (the default), 4317 and the agent's ports 2000, 4311, 4314, 4315, 4316, 8125, 25826 and 25888."
  }
}

# A JDK image: the workload is a single-file Java source program.
variable "java_image" {
  type    = string
  default = "public.ecr.aws/amazoncorretto/amazoncorretto:21-al2023"
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
