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
    name = "UseCase"
    type = "S"
  }

  attribute {
    name = "CommitHash"
    type = "S"
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
    name            = "UseCaseDate"
    hash_key        = "UseCase"
    range_key       = "CommitDate"
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