// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# VPC Endpoints for private subnet access
resource "aws_vpc_endpoint" "ecr_dkr" {
  vpc_id              = aws_vpc.efa_test_vpc.id
  service_name        = "com.amazonaws.us-west-2.ecr.dkr"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.efa_test_private_subnet[*].id
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name  = "efa-test-ecr-dkr-${module.common.testing_id}"
    Owner = "default"
  }
}

resource "aws_vpc_endpoint" "ecr_api" {
  vpc_id              = aws_vpc.efa_test_vpc.id
  service_name        = "com.amazonaws.us-west-2.ecr.api"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.efa_test_private_subnet[*].id
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name  = "efa-test-ecr-api-${module.common.testing_id}"
    Owner = "default"
  }
}

resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.efa_test_vpc.id
  service_name      = "com.amazonaws.us-west-2.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.efa_test_private_rt.id]

  tags = {
    Name  = "efa-test-s3-${module.common.testing_id}"
    Owner = "default"
  }
}

resource "aws_vpc_endpoint" "eks" {
  vpc_id              = aws_vpc.efa_test_vpc.id
  service_name        = "com.amazonaws.us-west-2.eks"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.efa_test_private_subnet[*].id
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true

  tags = {
    Name  = "efa-test-eks-${module.common.testing_id}"
    Owner = "default"
  }
}

# Security group for VPC endpoints
resource "aws_security_group" "vpc_endpoints" {
  name_prefix = "efa-test-vpc-endpoints-${module.common.testing_id}"
  vpc_id      = aws_vpc.efa_test_vpc.id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.efa_test_vpc.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name  = "efa-test-vpc-endpoints-sg-${module.common.testing_id}"
    Owner = "default"
  }
}
