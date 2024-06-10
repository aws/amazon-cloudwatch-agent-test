// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../common"
}

# Setup Dynamo Table for performance/stress testing
# Reference: https://github.com/aws/amazon-cloudwatch-agent-test/blob/b5e3217ab8cdce4a0eccab5db07856969f3a2fed/test/performancetest/transmitter.go#L75-L128
resource "aws_dynamodb_table" "performance-dynamodb-table" {
  name           = module.common.performance-dynamodb-table
  read_capacity  = 10
  write_capacity = 10
  hash_key       = "Service"
  range_key      = "UniqueID"

  attribute {
    name = "Service"
    type = "S"
  }

  attribute {
    name = "UniqueID"
    type = "S"
  }

  attribute {
    name = "CommitDate"
    type = "N"
  }

  attribute {
    name = "CommitHash"
    type = "S"
  }

  attribute {
    name = "UseCase"
    type = "S"
  }

  global_secondary_index {
    name            = "UseCaseDate"
    hash_key        = "UseCase"
    range_key       = "CommitDate"
    write_capacity  = 10
    read_capacity   = 10
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "UseCaseHash"
    hash_key        = "UseCase"
    range_key       = "CommitHash"
    write_capacity  = 10
    read_capacity   = 10
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "ServiceDate"
    hash_key        = "Service"
    range_key       = "CommitDate"
    write_capacity  = 10
    read_capacity   = 10
    projection_type = "ALL"
  }
}

## Setup Dedicated Host for Mac Resources
## https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ec2_host
## It is a requirement before creating an EC2 Mac Host
## https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-mac-instances.html
## Moreover, you can only place 1 mac instance on a dedicate host a single time.
## Therefore, limit down the scope for testing in Mac since EC2 can be done with Linux
## and Mac under the hood share similar plugins with Linux
## Mac has scrubbing to avoid patching 
## https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-mac-instances.html#mac-instance-stop
resource "aws_ec2_host" "dedicated_host" {
  ## Use lower bound and upper bound rule for Mac (e.g lower bound with the oldest version of Mac we support MacOs Catalina
  ## and upper bound for the newest version of Mac we support MacOS Ventura)
  for_each = {
    "Ventura_x86_64" : "mac1.metal"
    "Sonoma_x86_64" : "mac1.metal"
    "Ventura_arm64" : "mac2.metal"
    "Sonoma_arm64" : "mac2.metal"
  }

  ## Map 4x1 for avoid claimng resources
  instance_type     = each.value
  availability_zone = "${var.region}b"
  auto_placement    = "on"
}
