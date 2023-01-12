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
  default = "./integration/test/ecs/ecs_metadata"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}