// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# VPC for EFA integration test
resource "aws_vpc" "efa_test_vpc" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name  = "efa-test-vpc-${module.common.testing_id}"
    Owner = "default"
  }
}

# Internet Gateway
resource "aws_internet_gateway" "efa_test_igw" {
  vpc_id = aws_vpc.efa_test_vpc.id

  tags = {
    Name  = "efa-test-igw-${module.common.testing_id}"
    Owner = "default"
  }
}

# Public Subnets
resource "aws_subnet" "efa_test_public_subnet" {
  count = 2

  vpc_id                  = aws_vpc.efa_test_vpc.id
  cidr_block              = "10.0.${count.index + 1}.0/24"
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name  = "efa-test-public-subnet-${count.index + 1}-${module.common.testing_id}"
    Owner = "default"
  }
}

# Route Table for Public Subnets
resource "aws_route_table" "efa_test_public_rt" {
  vpc_id = aws_vpc.efa_test_vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.efa_test_igw.id
  }

  tags = {
    Name  = "efa-test-public-rt-${module.common.testing_id}"
    Owner = "default"
  }
}

# Associate Route Table with Public Subnets
resource "aws_route_table_association" "efa_test_public_rta" {
  count = length(aws_subnet.efa_test_public_subnet)

  subnet_id      = aws_subnet.efa_test_public_subnet[count.index].id
  route_table_id = aws_route_table.efa_test_public_rt.id
}

# Security Group
resource "aws_security_group" "efa_test_sg" {
  name_prefix = "efa-test-sg-${module.common.testing_id}"
  vpc_id      = aws_vpc.efa_test_vpc.id

  ingress {
    from_port = 0
    to_port   = 65535
    protocol  = "tcp"
    self      = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name  = "efa-test-sg-${module.common.testing_id}"
    Owner = "default"
  }
}

# Private Subnets
resource "aws_subnet" "efa_test_private_subnet" {
  count = 2

  vpc_id            = aws_vpc.efa_test_vpc.id
  cidr_block        = "10.0.${count.index + 10}.0/24"
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = {
    Name  = "efa-test-private-subnet-${count.index + 1}-${module.common.testing_id}"
    Owner = "default"
  }
}

# Elastic IP for NAT Gateway
resource "aws_eip" "efa_test_nat_eip" {
  domain = "vpc"

  tags = {
    Name  = "efa-test-nat-eip-${module.common.testing_id}"
    Owner = "default"
  }
}

# NAT Gateway
resource "aws_nat_gateway" "efa_test_nat" {
  allocation_id = aws_eip.efa_test_nat_eip.id
  subnet_id     = aws_subnet.efa_test_public_subnet[0].id

  tags = {
    Name  = "efa-test-nat-${module.common.testing_id}"
    Owner = "default"
  }

  depends_on = [aws_internet_gateway.efa_test_igw]
}

# Route Table for Private Subnets
resource "aws_route_table" "efa_test_private_rt" {
  vpc_id = aws_vpc.efa_test_vpc.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.efa_test_nat.id
  }

  tags = {
    Name  = "efa-test-private-rt-${module.common.testing_id}"
    Owner = "default"
  }
}

# Associate Route Table with Private Subnets
resource "aws_route_table_association" "efa_test_private_rta" {
  count = length(aws_subnet.efa_test_private_subnet)

  subnet_id      = aws_subnet.efa_test_private_subnet[count.index].id
  route_table_id = aws_route_table.efa_test_private_rt.id
}

# Data source for availability zones
data "aws_availability_zones" "available" {
  state = "available"
}
