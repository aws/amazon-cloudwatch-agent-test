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
  // Even though CommitDate would be better for a range key to easily sort; however, query without a hashkey is impossible.
  hash_key       = "CommitDate"
  range_key      = "CommitHash"

  attribute {
    name = "CommitHash"
    type = "S"
  }

  attribute {
    name = "CommitDate"
    type = "N"
  }
}