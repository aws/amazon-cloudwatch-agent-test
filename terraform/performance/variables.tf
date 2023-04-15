variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ec2_instance_type" {
  type    = string
  default = "t3a.xlarge"
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "ami" {
  type    = string
<<<<<<< HEAD
  default = "cloudwatch-agent-integration-test-win-2019*"
=======
  default = "cloudwatch-agent-integration-test-al2*"
>>>>>>> resources
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = "ec2-user"
}

<<<<<<< HEAD
=======
variable "arc" {
  type    = string
  default = "amd64"
}

>>>>>>> resources
variable "s3_bucket" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = "../../test/stress/statsd"
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "cwa_github_sha_date" {
  type    = string
  default = ""
}

variable "values_per_minute" {
  type    = number
  default = 10
}
