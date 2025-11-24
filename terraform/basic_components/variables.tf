variable "region" {
  default = "us-west-2"
}

variable "vpc_name" {
  type    = string
  default = ""
  description = "Name of an existing VPC to use. If empty, uses default VPC"
}
