variable "s3_bucket" {
  type    = string
  default = ""
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

variable "test_dir" {
  type    = string
  default = ""
}

variable "temp_directory" {
  type    = string
  default = ""
}

variable "arc" {
  type    = string
  default = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.arc)
    error_message = "Valid values for arc are (amd64, arm64)."
  }
}

variable "action" {
  type    = string
  default = "upload"

  validation {
    condition     = contains(["upload", "validate"], var.action)
    error_message = "Valid values for action are (upload, validate)."
  }
}

variable "family" {
  type    = string
  default = "linux"

  validation {
    condition     = contains(["windows", "darwin", "linux"], var.family)
    error_message = "Valid values for family are (windows, darwin, linux)."
  }
}


