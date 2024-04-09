variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ami" {
  type    = string
  default = "cloudwatch-agent-integration-test-ubuntu*"
}

variable "ec2_instance_type" {
  type    = string
  default = "t3a.xlarge"
}

variable "test_dir" {
  type    = string
  default = "./test/metric_value_benchmark"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

variable "metadataEnabled" {
  type    = string
  default = "enabled"
}

variable "hop_limit" {
  type    = number
  default = 2
}