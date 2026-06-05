variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "region" {
  type    = string
  default = "asia-southeast1"
}

variable "zone" {
  type    = string
  default = "asia-southeast1-a"
}

variable "machine_type" {
  type    = string
  default = "e2-small"
}

variable "ssh_user" {
  type        = string
  description = "Linux username created on the VM"
}

variable "ssh_pubkey_path" {
  type        = string
  description = "Path to the SSH public key file"
}

variable "allowed_ssh_cidr" {
  type        = string
  description = "CIDR allowed to SSH, e.g. 1.2.3.4/32"
}

variable "repo_url" {
  type        = string
  description = "Git URL the VM clones into /opt/wearwhere"
}

variable "bucket_name" {
  type        = string
  description = "Globally-unique GCS bucket for images + backups"
}
