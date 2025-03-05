#
# Copyright (c) 2023 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4.20.0"
    }
    rhcs = {
      version = ">= 1.6.3"
      source  = "terraform-redhat/rhcs"
    }
  }
}

# Export token using the RHCS_TOKEN environment variable
variable "rhcs_token" {
}
provider "rhcs" {
  token = var.rhcs_token
}

provider "aws" {
  region = var.aws_region
  ignore_tags {
    key_prefixes = ["kubernetes.io/"]
  }
  default_tags {
    tags = var.default_aws_tags
  }
}

data "aws_caller_identity" "current" {}
data "aws_availability_zones" "available" {}

locals {
  # Extract availability zone names for the specified region, limit it to 3 if multi-az or 1 if single
  region_azs = var.multi_az ? slice([for zone in data.aws_availability_zones.available.names : format("%s", zone)], 0, 3) : slice([for zone in data.aws_availability_zones.available.names : format("%s", zone)], 0, 1)
  account_id = data.aws_caller_identity.current.account_id
}

resource "random_string" "random_name" {
  length  = 6
  special = false
  upper   = false
}

locals {
  worker_node_replicas = var.multi_az ? 3 : 2
  # If cluster_name is not null, use that, otherwise generate a random cluster name
  cluster_name = coalesce(var.cluster_name, "cwa-rosa-test-${random_string.random_name.result}")
}

# The network validator requires an additional 60 seconds to validate Terraform clusters.
resource "time_sleep" "wait_60_seconds" {
  count = var.create_vpc ? 1 : 0
  depends_on = [module.vpc]
  create_duration = "60s"
}

module "hcp" {
  source                 = "terraform-redhat/rosa-hcp/rhcs"
  version                = "1.6.5"
  openshift_version      = "4.17.14"

  cluster_name           = local.cluster_name
  replicas               = local.worker_node_replicas
  aws_availability_zones = local.region_azs
  private                = var.private_cluster
  aws_subnet_ids         = var.create_vpc ? var.private_cluster ? module.vpc[0].private_subnets : concat(module.vpc[0].public_subnets, module.vpc[0].private_subnets) : var.aws_subnet_ids

  create_oidc            = true
  create_account_roles   = true
  account_role_prefix    = local.cluster_name
  create_operator_roles  = true
  operator_role_prefix   = local.cluster_name
  create_admin_user = true

  aws_billing_account_id     = var.billing_account_id
  ec2_metadata_http_tokens   = "required"

  depends_on = [time_sleep.wait_60_seconds]
}


############################
# HTPASSWD IDP
############################

resource "aws_secretsmanager_secret" "secret" {
  name = "${local.cluster_name}-htpasswd"

  tags = {
    Environment = "Production"
  }
}
resource "aws_secretsmanager_secret_version" "secret_version" {
  secret_id     = aws_secretsmanager_secret.secret.id
  secret_string = jsonencode({
    "openshift_password": module.hcp.cluster_admin_password
    "openshift_username": module.hcp.cluster_admin_username
    "openshift_server": module.hcp.cluster_api_url
  })
}

############################
# Setup CWA IAM
############################


locals {
  cloudwatch_agent_role_arn = lookup(module.hcp.account_roles_arn, "HCP-ROSA-Worker")
}
# Output the ARN of the CloudWatch Agent IAM role
output "cloudwatch_agent_role_arn" {
  value       = local.cloudwatch_agent_role_arn
  description = "ARN of the IAM role created for the CloudWatch agent"
}

# Attach CloudWatchAgentServerPolicy to the role
resource "aws_iam_role_policy_attachment" "cloudwatch_agent_policy" {
  role       = split("/", local.cloudwatch_agent_role_arn)[1]
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}
# Attach CloudWatchAgentAppSignalPolicy to the role
resource "aws_iam_role_policy_attachment" "cloudwatch_agent_appsig_policy" {
  role       = split("/", local.cloudwatch_agent_role_arn)[1]
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchApplicationSignalsFullAccess"
}
