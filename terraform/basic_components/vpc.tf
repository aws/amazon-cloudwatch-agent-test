// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# Create IPv6 VPC when ip_family is ipv6 and vpc_name is empty
resource "aws_vpc" "ipv6_vpc" {
  count = var.create_ipv6_vpc ? 1 : 0

  cidr_block                       = "10.0.0.0/16"
  assign_generated_ipv6_cidr_block = true
  enable_dns_hostnames             = true
  enable_dns_support               = true

  tags = {
    Name = "cwagent-ipv6-vpc-${module.common.testing_id}"
  }
}

# Create public subnets with IPv6
resource "aws_subnet" "ipv6_public_subnet" {
  count = var.create_ipv6_vpc ? 2 : 0

  vpc_id                          = aws_vpc.ipv6_vpc[0].id
  cidr_block                      = cidrsubnet(aws_vpc.ipv6_vpc[0].cidr_block, 8, count.index)
  ipv6_cidr_block                 = cidrsubnet(aws_vpc.ipv6_vpc[0].ipv6_cidr_block, 8, count.index)
  assign_ipv6_address_on_creation = true
  map_public_ip_on_launch         = true
  availability_zone               = data.aws_availability_zones.available.names[count.index]

  tags = {
    Name = "cwagent-ipv6-public-subnet-${count.index + 1}-${module.common.testing_id}"
  }
}

# Internet Gateway for IPv6 VPC
resource "aws_internet_gateway" "ipv6_igw" {
  count = var.create_ipv6_vpc ? 1 : 0

  vpc_id = aws_vpc.ipv6_vpc[0].id

  tags = {
    Name = "cwagent-ipv6-igw-${module.common.testing_id}"
  }
}

# Route table for public subnets
resource "aws_route_table" "ipv6_public_rt" {
  count = var.create_ipv6_vpc ? 1 : 0

  vpc_id = aws_vpc.ipv6_vpc[0].id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.ipv6_igw[0].id
  }

  route {
    ipv6_cidr_block = "::/0"
    gateway_id      = aws_internet_gateway.ipv6_igw[0].id
  }

  tags = {
    Name = "cwagent-ipv6-public-rt-${module.common.testing_id}"
  }
}

# Associate route table with subnets
resource "aws_route_table_association" "ipv6_public_rta" {
  count = var.create_ipv6_vpc ? 2 : 0

  subnet_id      = aws_subnet.ipv6_public_subnet[count.index].id
  route_table_id = aws_route_table.ipv6_public_rt[0].id
}

# Data source for availability zones
data "aws_availability_zones" "available" {
  state = "available"
}
