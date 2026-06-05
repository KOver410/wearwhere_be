# Bucket name is supplied at init time:
#   terraform init -backend-config="bucket=<YOUR_PROJECT_ID>-tfstate"
terraform {
  backend "gcs" {
    prefix = "wearwhere/state"
  }
}
