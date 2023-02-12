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
  range_key      = "CommitDate"

  attribute {
    name = "Service"
    type = "S"
  }

  attribute {
    name = "CommitHash"
    type = "S"
  }

  attribute {
    name = "CommitDate"
    type = "N"
  }

  global_secondary_index {
    name            = "CommitGithub"
    hash_key        = "CommitHash"
    range_key       = "CommitDate"
    write_capacity  = 10
    read_capacity   = 10
    projection_type = "ALL"
  }
}