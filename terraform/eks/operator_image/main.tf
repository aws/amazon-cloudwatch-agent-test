// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# Helm `set` entries that point the amazon-cloudwatch-observability chart at a
# custom operator image. With both inputs empty the list is empty, so the chart
# keeps its pinned operator. Shared by the EKS daemon suites so that every
# suite overrides the operator the same way.

variable "image_repo" {
  type        = string
  default     = ""
  description = "Operator image as <registry>/<repository>. Empty keeps the chart's operator repository."
  validation {
    condition     = var.image_repo == "" || can(regex("^[A-Za-z0-9.:-]+(/[A-Za-z0-9._-]+)+$", var.image_repo))
    error_message = "image_repo must be empty or <registry>/<repository>."
  }
}

variable "image_tag" {
  type        = string
  default     = ""
  description = "Operator image tag. Empty keeps the chart's operator tag."
}

variable "region" {
  type = string
}

output "helm_set" {
  # The chart composes the image as <repositoryDomainMap[region] or public>/<repository>:<tag>.
  value = concat(
    var.image_repo == "" ? [] : [
      { name = "manager.image.repositoryDomainMap.public", value = dirname(var.image_repo) },
      { name = "manager.image.repositoryDomainMap.${var.region}", value = dirname(var.image_repo) },
      { name = "manager.image.repository", value = basename(var.image_repo) },
    ],
    var.image_tag == "" ? [] : [
      { name = "manager.image.tag", value = var.image_tag },
    ],
  )
}
