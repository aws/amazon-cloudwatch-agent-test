// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

provider "aws" {
  region = var.region
}

# In this module, we are uploading setup's terraform state to s3 bucket for two reasons:
#-Share the setup configuration with others dev
#-Avoid creating duplicate resources when sharing the same account
# However, uncomment the terraform backend s3  if this is the second time doing the setup.

/*
terraform {
 backend "s3" {
    bucket           = "remote-terraform-state-s3-bucket"
    dynamodb_table   = "remote-terraform-state-dynamodb-table"
    key              = "terraform.tfstate"
    region           = "us-west-2"
    encrypt          = true
  }
}
*/



#Create S3 bucket to record terraform state for this setup in order to share the state for configuration setup when using integration test account
#Document: https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket
resource "aws_s3_bucket" "setup-remote-state-s3-bucket" {
  bucket = "remote-terraform-state-s3-bucket"
}

resource "aws_s3_bucket_server_side_encryption_configuration" "encrypt-setup-remote-state" {
  bucket = aws_s3_bucket.setup-remote-state-s3-bucket.bucket
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_acl" "acl-setup-remote-state" {
  bucket = aws_s3_bucket.setup-remote-state-s3-bucket.bucket
  acl    = "private"
}

resource "aws_s3_bucket_versioning" "versioning-setup-remote-state" {
  bucket = aws_s3_bucket.setup-remote-state-s3-bucket.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

#Avoid multiple developers change the state at the same time since it would cause race condition
#Document: https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/dynamodb_table
resource "aws_dynamodb_table" "setup-remote-state-dynamodb-table" {
  name         = "remote-terraform-state-dynamodb-table"
  hash_key     = "LockID"
  billing_mode = "PAY_PER_REQUEST"

  attribute {
    name = "LockID"
    type = "S"
  }

  depends_on = [aws_s3_bucket.setup-remote-state-s3-bucket]
}