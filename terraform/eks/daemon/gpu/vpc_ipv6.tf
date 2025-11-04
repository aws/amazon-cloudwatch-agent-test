# IPv6-enabled VPC for EKS
resource "aws_vpc" "ipv6_vpc" {
  count                            = var.ip_family == "ipv6" ? 1 : 0
  cidr_block                       = "10.0.0.0/16"
  enable_dns_hostnames             = true
  enable_dns_support               = true
  assign_generated_ipv6_cidr_block = true

  tags = {
    Name = "cwagent-eks-ipv6-vpc-${module.common.testing_id}"
  }
}

# Public Subnets with IPv6
resource "aws_subnet" "ipv6_public" {
  count                           = var.ip_family == "ipv6" ? 2 : 0
  vpc_id                          = aws_vpc.ipv6_vpc[0].id
  cidr_block                      = cidrsubnet(aws_vpc.ipv6_vpc[0].cidr_block, 8, count.index)
  ipv6_cidr_block                 = cidrsubnet(aws_vpc.ipv6_vpc[0].ipv6_cidr_block, 8, count.index)
  availability_zone               = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch         = true
  assign_ipv6_address_on_creation = true

  tags = {
    Name                                                    = "cwagent-eks-ipv6-public-${count.index}-${module.common.testing_id}"
    "kubernetes.io/role/elb"                                = "1"
    "kubernetes.io/cluster/cwagent-eks-integ-${module.common.testing_id}" = "shared"
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

# Internet Gateway
resource "aws_internet_gateway" "ipv6_igw" {
  count  = var.ip_family == "ipv6" ? 1 : 0
  vpc_id = aws_vpc.ipv6_vpc[0].id

  tags = {
    Name = "cwagent-eks-ipv6-igw-${module.common.testing_id}"
  }
}

# Route Table
resource "aws_route_table" "ipv6_public" {
  count  = var.ip_family == "ipv6" ? 1 : 0
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
    Name = "cwagent-eks-ipv6-public-rt-${module.common.testing_id}"
  }
}

# Route Table Association
resource "aws_route_table_association" "ipv6_public" {
  count          = var.ip_family == "ipv6" ? 2 : 0
  subnet_id      = aws_subnet.ipv6_public[count.index].id
  route_table_id = aws_route_table.ipv6_public[0].id
}

# Security Group for IPv6
resource "aws_security_group" "ipv6_sg" {
  count       = var.ip_family == "ipv6" ? 1 : 0
  name        = "cwagent-eks-ipv6-sg-${module.common.testing_id}"
  description = "Security group for IPv6 EKS cluster"
  vpc_id      = aws_vpc.ipv6_vpc[0].id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    ipv6_cidr_blocks = ["::/0"]
  }

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name = "cwagent-eks-ipv6-sg-${module.common.testing_id}"
  }
}

# Locals to use either IPv6 or IPv4 resources
locals {
  vpc_id             = var.ip_family == "ipv6" ? aws_vpc.ipv6_vpc[0].id : module.basic_components.vpc_id
  subnet_ids         = var.ip_family == "ipv6" ? aws_subnet.ipv6_public[*].id : module.basic_components.public_subnet_ids
  security_group_id  = var.ip_family == "ipv6" ? aws_security_group.ipv6_sg[0].id : module.basic_components.security_group
}
