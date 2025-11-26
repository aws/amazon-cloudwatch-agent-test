variable "region" {
  default = "us-west-2"
}

variable "vpc_name" {
  type        = string
  default     = ""
  description = "Name of an existing VPC to use. If empty, uses default VPC"
}

variable "ip_family" {
  type        = string
  default     = "ipv4"
  description = "IP family for the cluster. Use 'ipv6' to create an IPv6-enabled VPC"
}

variable "create_ipv6_vpc" {
  type        = bool
  default     = false
  description = "Whether to create a new IPv6 VPC. Set to true when ip_family is ipv6 and vpc_name is empty"
}
