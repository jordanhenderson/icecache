terraform {
  backend "s3" {
    bucket = "uc-terraform"
    key = "icecache/state/terraform.tfstate"
    encrypt = true
    region = "ap-southeast-2"
    dynamodb_table = "terraform-state-lock-table"
  }
}
