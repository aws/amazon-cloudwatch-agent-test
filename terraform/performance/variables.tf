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
  default = "cloudwatch-agent-integration-test-ubuntu*"
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = ""
}

variable "install_agent" {
  description = "go run ./install/install_agent.go deb or go run ./install/install_agent.go rpm"
  type        = string
  default     = "go run ./install/install_agent.go rpm"
}

variable "arc" {
  type    = string
  default = ""
}

variable "binary_name" {
  type    = string
  default = ""
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "test_name" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = ""
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "github_test_repo" {
  type    = string
  default = ""
}

variable "github_test_repo_branch" {
  type    = string
  default = "main"
}

variable "cwa_github_sha_date" {
  type    = string
  default = ""
}
variable "performance_number_of_logs" {
  type    = string
  default = "100"
}
