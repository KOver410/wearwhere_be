output "api_ip" {
  description = "Static external IP — point your DNS/DuckDNS here"
  value       = google_compute_address.api_ip.address
}

output "bucket" {
  description = "GCS bucket for images + backups"
  value       = google_storage_bucket.assets.name
}
