resource "google_compute_address" "api_ip" {
  name   = "wearwhere-api-ip"
  region = var.region
}

resource "google_compute_firewall" "web" {
  name          = "wearwhere-allow-web"
  network       = "default"
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["wearwhere-api"]
  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
}

resource "google_compute_firewall" "ssh" {
  name          = "wearwhere-allow-ssh"
  network       = "default"
  source_ranges = [var.allowed_ssh_cidr]
  target_tags   = ["wearwhere-api"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

resource "google_service_account" "vm_sa" {
  account_id   = "wearwhere-vm"
  display_name = "WearWhere VM service account"
}

resource "google_storage_bucket" "assets" {
  name                        = var.bucket_name
  location                    = var.region
  uniform_bucket_level_access = true
  force_destroy               = false

  lifecycle_rule {
    condition {
      age            = 30
      matches_prefix = ["backups/"]
    }
    action {
      type = "Delete"
    }
  }
}

resource "google_storage_bucket_iam_member" "vm_object_admin" {
  bucket = google_storage_bucket.assets.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.vm_sa.email}"
}

resource "google_compute_instance" "api" {
  name         = "wearwhere-api"
  machine_type = var.machine_type
  zone         = var.zone
  tags         = ["wearwhere-api"]

  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2204-lts"
      size  = 20
    }
  }

  network_interface {
    network = "default"
    access_config {
      nat_ip = google_compute_address.api_ip.address
    }
  }

  service_account {
    email  = google_service_account.vm_sa.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    ssh-keys       = "${var.ssh_user}:${file(var.ssh_pubkey_path)}"
    startup-script = templatefile("${path.module}/startup-script.sh", { repo_url = var.repo_url })
  }
}
