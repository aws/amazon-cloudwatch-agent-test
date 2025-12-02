variable "region" {
  default = "us-west-2"
}

variable "vpc_name" {
  type        = string
  default     = ""
  description = "Name of an existing VPC to use. If empty, uses default VPC"
}

variable "ip_family" {
  type    = string
  default = "ipv4"
  validation {
    condition     = contains(["ipv4", "ipv6"], var.ip_family)
    error_message = "ip_family must be either 'ipv4' or 'ipv6'"
  }
  description = "IP family for the cluster. Use 'ipv6' to filter for IPv6-enabled subnets"
}
