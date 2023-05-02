provider "aws" {
  region = var.region
}

provider "kubernetes" {
  load_config_file = false
}