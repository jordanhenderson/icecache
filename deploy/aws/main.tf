module "lambda_layer" {
  source = "./modules/lambda_layer"
  layer_name          = "icecache"
  description         = "A persistence layer for lambda ephemeral storage"
  license_info        = "Apache-2.0"
  compatible_runtimes = ["provided.al2023"]
  s3_bucket      = var.S3_BUCKET
  s3_key         = "layers/icecache.zip"
  local_zip_path = "../../dist/icecache.zip"
}