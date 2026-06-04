# GCP Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the WearWhere Go backend (API + PostgreSQL + Redis + object storage) to a single GCP Compute Engine VM running docker-compose, with infrastructure provisioned by Terraform, HTTPS via Caddy, and CORS enabled so the React web client can connect.

**Architecture:** One `e2-small` VM in `asia-southeast1` runs four containers (postgres, redis, api, caddy) via `docker-compose.prod.yml`. Terraform provisions the VM, static IP, firewall, GCS bucket, and a service account. A startup script installs Docker and clones the repo; a deploy script pulls and rebuilds. Demo uses local-disk storage + a free DuckDNS subdomain; production flips env vars to GCS storage + a real domain (see the cutover checklist task).

**Tech Stack:** Go 1.23 / Gin, Docker + docker-compose, Caddy 2 (auto-TLS), Terraform (Google provider), golang-migrate, GCS.

---

## File Structure

**New files:**
- `internal/shared/httpmw/cors.go` — CORS middleware constructor (only responsibility: build a `gin.HandlerFunc` from allowed origins)
- `internal/shared/httpmw/cors_test.go` — unit test for the above
- `Dockerfile` — multi-stage build of the API binary
- `.dockerignore` — keep build context small
- `docker-compose.prod.yml` — self-contained 4-service + migrate stack for the VM
- `deploy/Caddyfile` — reverse proxy + auto-HTTPS
- `deploy/terraform/providers.tf` — terraform + google provider config
- `deploy/terraform/backend.tf` — GCS state backend (partial config)
- `deploy/terraform/variables.tf` — input variables
- `deploy/terraform/main.tf` — IP, firewall, SA, bucket, VM
- `deploy/terraform/outputs.tf` — IP + bucket outputs
- `deploy/terraform/startup-script.sh` — VM bootstrap (Docker + clone)
- `deploy/terraform/terraform.tfvars.example` — sample variable values
- `deploy/deploy.sh` — pull + rebuild (run on VM)
- `deploy/backup.sh` — `pg_dump` → GCS (cron)
- `deploy/README.md` — runbook + production cutover checklist
- `.env.production.example` — production env template

**Modified files:**
- `internal/config/config.go` — add `CORSConfig`
- `cmd/api/main.go` — wire CORS middleware into the router
- `.env.example` — add `CORS_ALLOWED_ORIGINS`

---

## Phase A — Containerize app + CORS (verifiable locally, no cloud)

### Task 1: CORS middleware + config

**Files:**
- Create: `internal/shared/httpmw/cors.go`
- Create: `internal/shared/httpmw/cors_test.go`
- Modify: `internal/config/config.go`
- Modify: `cmd/api/main.go`
- Modify: `.env.example`

- [ ] **Step 1: Add the cors dependency**

Run (PowerShell, repo root):
```
go get github.com/gin-contrib/cors@v1.7.2
```
Expected: `go.mod` / `go.sum` updated, no errors.

- [ ] **Step 2: Write the failing test**

Create `internal/shared/httpmw/cors_test.go`:
```go
package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newEngine(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func TestCORS_AllowedOriginGetsHeader(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}

func TestCORS_PreflightReturns204(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
}

func TestCORS_DisallowedOriginNoEcho(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "https://evil.example.com" {
		t.Fatalf("must not echo disallowed origin, got %q", got)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```
go test ./internal/shared/httpmw/ -run TestCORS -v
```
Expected: FAIL — `undefined: CORS` (package/function doesn't exist yet).

- [ ] **Step 4: Implement the middleware**

Create `internal/shared/httpmw/cors.go`:
```go
// Package httpmw holds cross-cutting HTTP middleware shared across modules.
package httpmw

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS builds a gin middleware that allows the given browser origins.
//
// We use Bearer tokens (not cookies), so AllowCredentials stays false.
// When allowedOrigins is empty, all origins are allowed — a dev convenience;
// production MUST set CORS_ALLOWED_ORIGINS (enforced by the cutover checklist).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	c := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}
	if len(allowedOrigins) == 0 {
		c.AllowAllOrigins = true
	} else {
		c.AllowOrigins = allowedOrigins
	}
	return cors.New(c)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```
go test ./internal/shared/httpmw/ -run TestCORS -v
```
Expected: PASS (3 tests).

- [ ] **Step 6: Add CORSConfig to config**

In `internal/config/config.go`, add a field to the `Config` struct (after `Reservation ReservationConfig`):
```go
	CORS        CORSConfig
```
Add the type (next to the other config types, e.g. after `ReservationConfig`):
```go
type CORSConfig struct {
	AllowedOrigins []string
}
```
In `Load()`, after the `cfg.Reservation = ...` block and before `return cfg, nil`:
```go
	cfg.CORS = CORSConfig{
		AllowedOrigins: csvOrSingle("CORS_ALLOWED_ORIGINS", ""),
	}
```

- [ ] **Step 7: Wire the middleware into the router**

In `cmd/api/main.go`, add the import (with the other `internal/shared/...` imports):
```go
	"github.com/wearwhere/wearwhere_be/internal/shared/httpmw"
```
Then in the `// ── router ──` block, insert the CORS middleware right after the logger:
```go
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(httpmw.CORS(cfg.CORS.AllowedOrigins))
```

- [ ] **Step 8: Add the env var to .env.example**

In `.env.example`, under the `# ── Server ──` block add:
```
# Comma-separated browser origins allowed by CORS (React web, etc.).
# Leave blank in dev to allow all; MUST be set in production.
CORS_ALLOWED_ORIGINS=
```

- [ ] **Step 9: Build + run full unit suite**

Run:
```
go build ./... ; go test ./internal/shared/httpmw/ ./internal/config/ -v
```
Expected: build succeeds; tests PASS.

- [ ] **Step 10: Commit**

```
git add internal/shared/httpmw/ internal/config/config.go cmd/api/main.go .env.example go.mod go.sum
git commit -m "feat(http): CORS middleware configurable via CORS_ALLOWED_ORIGINS"
```

---

### Task 2: Dockerfile for the API

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

- [ ] **Step 1: Create `.dockerignore`**

```
.git/
docs/
*.md
*.exe
bin/
uploads/
.env
.env.*
.beads/
SRS_wearwhere.pdf
```

- [ ] **Step 2: Create `Dockerfile`**

```dockerfile
# ---- build stage ----
FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/api /app/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/api"]
```

- [ ] **Step 3: Build the image to verify it compiles in Docker**

Run (Docker Desktop must be running):
```
docker build -t wearwhere-api:dev .
```
Expected: build completes, final image tagged `wearwhere-api:dev`.

- [ ] **Step 4: Commit**

```
git add Dockerfile .dockerignore
git commit -m "build: multi-stage Dockerfile for API"
```

---

### Task 3: Production compose stack + Caddy

**Files:**
- Create: `docker-compose.prod.yml`
- Create: `deploy/Caddyfile`
- Create: `.env.production.example`

- [ ] **Step 1: Create `deploy/Caddyfile`**

```
{$SITE_ADDRESS} {
	encode gzip
	reverse_proxy api:8080
}
```
(Caddy auto-provisions HTTPS for `$SITE_ADDRESS` via Let's Encrypt when it is a real domain.)

- [ ] **Step 2: Create `docker-compose.prod.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    volumes:
      - redisdata:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  migrate:
    image: migrate/migrate:v4.17.1
    restart: on-failure
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ./db/migrations:/migrations
    command: ["-path", "/migrations", "-database", "${DATABASE_URL}", "up"]

  api:
    build:
      context: .
      dockerfile: Dockerfile
    restart: unless-stopped
    env_file: .env
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - uploads:/app/uploads
    expose:
      - "8080"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    depends_on:
      - api
    ports:
      - "80:80"
      - "443:443"
    environment:
      SITE_ADDRESS: ${SITE_ADDRESS}
    volumes:
      - ./deploy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config

volumes:
  pgdata:
  redisdata:
  uploads:
  caddy_data:
  caddy_config:
```

- [ ] **Step 3: Create `.env.production.example`**

```
# ── Server ─────────────────────────────────────────────
APP_ENV=production
HTTP_PORT=8080

# ── PostgreSQL (compose interpolation + app) ───────────
POSTGRES_USER=wearwhere
POSTGRES_PASSWORD=CHANGE_ME_STRONG
POSTGRES_DB=wearwhere
DATABASE_URL=postgres://wearwhere:CHANGE_ME_STRONG@postgres:5432/wearwhere?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# ── Redis ──────────────────────────────────────────────
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# ── JWT (generate: openssl rand -base64 64) ────────────
JWT_SECRET=GENERATE_A_LONG_RANDOM_SECRET
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h

# ── Caddy site + CORS ──────────────────────────────────
SITE_ADDRESS=wearwhere.duckdns.org
CORS_ALLOWED_ORIGINS=https://wearwhere.duckdns.org,http://localhost:3000

# ── Storage (demo=local; prod set STORAGE_DRIVER=gcs) ──
STORAGE_DRIVER=local
STORAGE_LOCAL_DIR=/app/uploads
STORAGE_BASE_URL=https://wearwhere.duckdns.org/uploads
STORAGE_GCS_BUCKET=
STORAGE_GCS_CREDENTIALS=
STORAGE_MAX_FILE_SIZE=5242880

# ── Backups (used by deploy/backup.sh) ─────────────────
BUCKET=

# ── SMTP / SMS / OAuth / PayOS / Goship ────────────────
# Copy remaining keys from .env.example and fill per the
# production cutover checklist (deploy/README.md). Demo defaults:
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM_EMAIL=no-reply@wearwhere.app
SMTP_FROM_NAME=WearWhere
TWILIO_ACCOUNT_SID=
TWILIO_AUTH_TOKEN=
TWILIO_FROM_NUMBER=
GOOGLE_CLIENT_IDS=
APPLE_CLIENT_IDS=
LOGIN_MAX_ATTEMPTS=5
LOGIN_LOCKOUT_MINUTES=15
OTP_TTL_MINUTES=15
OTP_MAX_PER_HOUR=3
RATE_LIMIT_PER_MIN=100
PAYOS_MODE=mock
PAYOS_CLIENT_ID=
PAYOS_API_KEY=
PAYOS_CHECKSUM_KEY=
PAYOS_RETURN_URL=https://wearwhere.duckdns.org/checkout/success
PAYOS_CANCEL_URL=https://wearwhere.duckdns.org/checkout/cancel
PAYOS_BASE_URL=https://wearwhere.duckdns.org
SHIPPING_PROVIDER=flat
GOSHIP_MODE=mock
GOSHIP_TOKEN=
GOSHIP_BASE_URL=https://sandbox.goship.io/api/v2
GOSHIP_DEFAULT_ITEM_WEIGHT_G=500
GOSHIP_DEFAULT_LENGTH_CM=20
GOSHIP_DEFAULT_WIDTH_CM=15
GOSHIP_DEFAULT_HEIGHT_CM=10
RESERVATION_TIMEOUT_MINUTES=30
RESERVATION_CLEANUP_INTERVAL=5m
```

- [ ] **Step 4: Smoke-test the stack locally (HTTP only)**

Create a throwaway local env and run the stack without Caddy TLS. Run (PowerShell, repo root):
```
Copy-Item .env.production.example .env.localtest
```
Edit `.env.localtest`: set `SITE_ADDRESS=:80` (forces Caddy to serve plain HTTP, no cert) and `JWT_SECRET` to any non-empty value. Then:
```
docker compose -f docker-compose.prod.yml --env-file .env.localtest up -d --build
```
Wait ~20s, then:
```
curl http://localhost/healthz
```
Expected: `{"status":"ok"}` (request flows curl → caddy → api). The `migrate` container should show `Exited (0)` in `docker compose -f docker-compose.prod.yml ps`.

- [ ] **Step 5: Tear down the local test**

```
docker compose -f docker-compose.prod.yml --env-file .env.localtest down -v
Remove-Item .env.localtest
```

- [ ] **Step 6: Commit**

```
git add docker-compose.prod.yml deploy/Caddyfile .env.production.example
git commit -m "build: production compose stack (api+caddy+migrate) and env template"
```

---

## Phase B — Provision GCP infrastructure (Terraform)

### Task 4: GCP prerequisites (manual, one-time)

**Files:** none (account/tooling setup).

- [ ] **Step 1: Install tools**

Install the Google Cloud SDK and Terraform on your Windows machine:
```
winget install --id Google.CloudSDK -e
winget install --id Hashicorp.Terraform -e
```
Verify (new terminal):
```
gcloud version ; terraform version
```
Expected: both print versions.

- [ ] **Step 2: Create a GCP project + start the free trial**

1. Go to https://console.cloud.google.com → create a project, e.g. `wearwhere-prod`.
2. Activate the **$300 / 90-day free trial** (billing → "Start free trial"). A billing account is required even for free-tier; the trial card is not charged until you manually upgrade.
3. Note the **Project ID** (not the display name).

- [ ] **Step 3: Authenticate and set the project**

Run:
```
gcloud auth login
gcloud auth application-default login
gcloud config set project <YOUR_PROJECT_ID>
```
Expected: browser auth completes; `gcloud config list` shows your project.

- [ ] **Step 4: Enable required APIs**

```
gcloud services enable compute.googleapis.com storage.googleapis.com iam.googleapis.com
```
Expected: `Operation finished successfully`.

- [ ] **Step 5: Create the Terraform state bucket**

Pick a globally-unique name (buckets are global). Run:
```
gcloud storage buckets create gs://<YOUR_PROJECT_ID>-tfstate --location=asia-southeast1 --uniform-bucket-level-access
```
Expected: bucket created. Record this name — it is the `-backend-config` value in Task 6.

- [ ] **Step 6: Generate an SSH key (if you don't have one)**

```
ssh-keygen -t ed25519 -C "wearwhere" -f $env:USERPROFILE\.ssh\wearwhere
```
Expected: creates `wearwhere` (private) and `wearwhere.pub` (public).

- [ ] **Step 7: Find your public IP for SSH firewall**

Open https://ifconfig.me — record the IP as `<YOUR_IP>/32` for `allowed_ssh_cidr`.

---

### Task 5: Terraform configuration files

**Files:**
- Create: `deploy/terraform/providers.tf`
- Create: `deploy/terraform/backend.tf`
- Create: `deploy/terraform/variables.tf`
- Create: `deploy/terraform/outputs.tf`
- Create: `deploy/terraform/terraform.tfvars.example`

- [ ] **Step 1: Create `deploy/terraform/providers.tf`**

```hcl
terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}
```

- [ ] **Step 2: Create `deploy/terraform/backend.tf`**

```hcl
# Bucket name is supplied at init time:
#   terraform init -backend-config="bucket=<YOUR_PROJECT_ID>-tfstate"
terraform {
  backend "gcs" {
    prefix = "wearwhere/state"
  }
}
```

- [ ] **Step 3: Create `deploy/terraform/variables.tf`**

```hcl
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
```

- [ ] **Step 4: Create `deploy/terraform/outputs.tf`**

```hcl
output "api_ip" {
  description = "Static external IP — point your DNS/DuckDNS here"
  value       = google_compute_address.api_ip.address
}

output "bucket" {
  description = "GCS bucket for images + backups"
  value       = google_storage_bucket.assets.name
}
```

- [ ] **Step 5: Create `deploy/terraform/terraform.tfvars.example`**

```hcl
project_id       = "wearwhere-prod"
ssh_user         = "deploy"
ssh_pubkey_path  = "C:/Users/PC/.ssh/wearwhere.pub"
allowed_ssh_cidr = "1.2.3.4/32"
repo_url         = "https://github.com/<you>/wearwhere_be.git"
bucket_name      = "wearwhere-prod-assets"
```

- [ ] **Step 6: Commit**

```
git add deploy/terraform/providers.tf deploy/terraform/backend.tf deploy/terraform/variables.tf deploy/terraform/outputs.tf deploy/terraform/terraform.tfvars.example
git commit -m "infra: terraform provider, backend, variables, outputs"
```

---

### Task 6: Terraform resources + startup script

**Files:**
- Create: `deploy/terraform/main.tf`
- Create: `deploy/terraform/startup-script.sh`

- [ ] **Step 1: Create `deploy/terraform/startup-script.sh`**

```bash
#!/usr/bin/env bash
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y ca-certificates curl git

# Docker official repo
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker

# First-time clone (idempotent)
if [ ! -d /opt/wearwhere ]; then
  git clone ${repo_url} /opt/wearwhere
fi
```
(Only `${repo_url}` is a Terraform template token; all shell expansions use `$(...)`/`$VAR` without braces, so `templatefile` leaves them intact.)

- [ ] **Step 2: Create `deploy/terraform/main.tf`**

```hcl
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
```

- [ ] **Step 3: Initialize Terraform**

Run (PowerShell, in `deploy/terraform`):
```
terraform init -backend-config="bucket=<YOUR_PROJECT_ID>-tfstate"
```
Expected: `Terraform has been successfully initialized!`

- [ ] **Step 4: Validate + plan**

```
Copy-Item terraform.tfvars.example terraform.tfvars
```
Edit `terraform.tfvars` with your real values (project_id, ssh_user, pubkey path, your `/32` IP, repo_url, unique bucket_name). Then:
```
terraform validate
terraform plan
```
Expected: `validate` → "Success"; `plan` shows **7 resources to add** (IP, 2 firewalls, SA, bucket, IAM member, instance), no errors.

- [ ] **Step 5: Add terraform.tfvars to .gitignore + commit**

Append to repo-root `.gitignore` (create if missing):
```
deploy/terraform/.terraform/
deploy/terraform/terraform.tfvars
*.tfstate
*.tfstate.*
```
Commit:
```
git add deploy/terraform/main.tf deploy/terraform/startup-script.sh .gitignore
git commit -m "infra: terraform VM/firewall/bucket/SA + VM bootstrap script"
```

---

### Task 7: Apply infrastructure + DNS

**Files:** none (provisioning + external DNS).

- [ ] **Step 1: Apply**

Run (in `deploy/terraform`):
```
terraform apply
```
Type `yes`. Expected: 7 resources created; outputs print `api_ip` and `bucket`.

- [ ] **Step 2: Record the IP**

```
terraform output api_ip
```
Record the IP, e.g. `34.xx.xx.xx`.

- [ ] **Step 3: Point DuckDNS at the IP**

1. Go to https://www.duckdns.org, sign in, create a subdomain e.g. `wearwhere`.
2. Set its **current ip** to the `api_ip` value → Update.
3. Verify (PowerShell):
```
nslookup wearwhere.duckdns.org
```
Expected: resolves to your `api_ip`.

- [ ] **Step 4: Verify SSH + Docker on the VM**

```
ssh -i $env:USERPROFILE\.ssh\wearwhere <ssh_user>@<api_ip> "docker --version && ls /opt/wearwhere"
```
Expected: prints a Docker version and the repo file listing. (If the repo isn't cloned yet, wait ~60s for the startup script and retry.)

---

## Phase C — Deploy + operate

### Task 8: First deployment on the VM

**Files:** none (executed on the VM).

- [ ] **Step 1: SSH in and create the production `.env`**

```
ssh -i $env:USERPROFILE\.ssh\wearwhere <ssh_user>@<api_ip>
```
On the VM:
```
cd /opt/wearwhere
cp .env.production.example .env
nano .env
```
Set at minimum: a strong `POSTGRES_PASSWORD` (and the same password inside `DATABASE_URL`), a freshly generated `JWT_SECRET` (`openssl rand -base64 64`), `SITE_ADDRESS=wearwhere.duckdns.org`, `CORS_ALLOWED_ORIGINS=https://wearwhere.duckdns.org,http://localhost:3000`, and `BUCKET=<your bucket_name>`. Then:
```
chmod 600 .env
```

- [ ] **Step 2: Bring the stack up**

```
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
```
Expected: `postgres`, `redis`, `api`, `caddy` are `Up`; `migrate` is `Exited (0)`.

- [ ] **Step 3: Verify HTTPS + health from your laptop**

From PowerShell (not the VM):
```
curl https://wearwhere.duckdns.org/healthz
```
Expected: `{"status":"ok"}` over a valid certificate (Caddy obtained Let's Encrypt automatically). If you get a TLS error, wait ~30s for cert issuance and retry.

- [ ] **Step 4: Verify CORS works for a browser origin**

```
curl -i -X OPTIONS https://wearwhere.duckdns.org/api/v1/products `
  -H "Origin: http://localhost:3000" `
  -H "Access-Control-Request-Method: GET"
```
Expected: `HTTP/2 204` with `access-control-allow-origin: http://localhost:3000` in the headers.

- [ ] **Step 5: Verify migrations ran**

On the VM:
```
docker compose -f docker-compose.prod.yml exec -T postgres psql -U wearwhere -d wearwhere -c "\dt" | head
```
Expected: lists application tables (users, products, orders, ...).

---

### Task 9: Deploy/update script

**Files:**
- Create: `deploy/deploy.sh`

- [ ] **Step 1: Create `deploy/deploy.sh`**

```bash
#!/usr/bin/env bash
# Pull latest code and (re)build the stack on the VM.
set -euo pipefail
cd /opt/wearwhere
git pull --ff-only
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
echo "deployed commit: $(git rev-parse --short HEAD)"
```

- [ ] **Step 2: Commit (from your laptop)**

```
git add deploy/deploy.sh
git commit -m "ops: deploy/update script for the VM"
git push
```

- [ ] **Step 3: Test the update flow on the VM**

```
ssh -i $env:USERPROFILE\.ssh\wearwhere <ssh_user>@<api_ip> "cd /opt/wearwhere && chmod +x deploy/deploy.sh && ./deploy/deploy.sh"
```
Expected: `git pull` succeeds, stack rebuilds, prints the deployed commit. Re-run `curl https://wearwhere.duckdns.org/healthz` → still `ok`.

---

### Task 10: Daily database backup to GCS

**Files:**
- Create: `deploy/backup.sh`

- [ ] **Step 1: Create `deploy/backup.sh`**

```bash
#!/usr/bin/env bash
# Dump Postgres and upload to gs://$BUCKET/backups/. Run via cron.
# Uses the VM service account (Application Default Credentials on GCE)
# through the google/cloud-sdk container — no gcloud install on host needed.
set -euo pipefail
set -a
source /opt/wearwhere/.env
set +a
cd /opt/wearwhere

TS=$(date +%Y%m%d-%H%M%S)
FILE=/tmp/wearwhere-$TS.sql.gz

docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$FILE"

docker run --rm -v /tmp:/tmp google/cloud-sdk:slim \
  gcloud storage cp "$FILE" "gs://$BUCKET/backups/"

rm -f "$FILE"
echo "backup uploaded: gs://$BUCKET/backups/$(basename "$FILE")"
```
(Old backups are auto-deleted after 30 days by the bucket lifecycle rule from Task 6.)

- [ ] **Step 2: Commit + push (from laptop)**

```
git add deploy/backup.sh
git commit -m "ops: daily pg_dump backup to GCS"
git push
```

- [ ] **Step 3: Install on the VM and run once**

```
ssh -i $env:USERPROFILE\.ssh\wearwhere <ssh_user>@<api_ip>
```
On the VM:
```
cd /opt/wearwhere && git pull --ff-only && chmod +x deploy/backup.sh
sudo ./deploy/backup.sh
```
Expected: prints `backup uploaded: gs://<bucket>/backups/wearwhere-....sql.gz`. Verify:
```
docker run --rm google/cloud-sdk:slim gcloud storage ls gs://<bucket>/backups/
```
Expected: lists the dump file.

- [ ] **Step 4: Schedule daily via cron (02:00 VN = 19:00 UTC)**

On the VM:
```
( sudo crontab -l 2>/dev/null; echo "0 19 * * * /opt/wearwhere/deploy/backup.sh >> /var/log/wearwhere-backup.log 2>&1" ) | sudo crontab -
sudo crontab -l
```
Expected: the cron line appears.

---

## Phase D — Production cutover

### Task 11: Production cutover runbook

**Files:**
- Create: `deploy/README.md`

- [ ] **Step 1: Create `deploy/README.md`**

````markdown
# WearWhere Deployment Runbook

Single GCP Compute Engine VM running `docker-compose.prod.yml`
(postgres + redis + api + caddy). Infra in `deploy/terraform/`.

## First deploy
See `docs/superpowers/plans/2026-06-04-gcp-deployment.md` (Tasks 4–8).

## Update
```
ssh -i ~/.ssh/wearwhere <user>@<ip>
cd /opt/wearwhere && ./deploy/deploy.sh
```

## Restore a backup
```
cd /opt/wearwhere
docker run --rm -v /tmp:/tmp google/cloud-sdk:slim \
  gcloud storage cp gs://$BUCKET/backups/<file>.sql.gz /tmp/restore.sql.gz
gunzip -c /tmp/restore.sql.gz | \
  docker compose -f docker-compose.prod.yml exec -T postgres psql -U $POSTGRES_USER -d $POSTGRES_DB
```

## Production cutover checklist (demo → prod)

Edit `/opt/wearwhere/.env` on the VM, then `./deploy/deploy.sh`:

- [ ] `APP_ENV=production`
- [ ] `JWT_SECRET` regenerated (`openssl rand -base64 64`)
- [ ] `CORS_ALLOWED_ORIGINS` set to real web origin(s) — NOT blank
- [ ] Real domain: update DNS A record → `api_ip`; set `SITE_ADDRESS`,
      `STORAGE_BASE_URL`, `PAYOS_RETURN_URL`, `PAYOS_CANCEL_URL`, `PAYOS_BASE_URL`
- [ ] PayOS: register individual account (CCCD + personal bank account in your name),
      set `PAYOS_MODE=production` + `PAYOS_CLIENT_ID` / `PAYOS_API_KEY` / `PAYOS_CHECKSUM_KEY`
- [ ] Goship: `SHIPPING_PROVIDER=goship`, `GOSHIP_MODE=production` (or sandbox) + `GOSHIP_TOKEN`
- [ ] OAuth: fill `GOOGLE_CLIENT_IDS` / `APPLE_CLIENT_IDS` (web + iOS + Android)
- [ ] SMTP: real Gmail app password
- [ ] Storage → GCS: `STORAGE_DRIVER=gcs`, `STORAGE_GCS_BUCKET=<bucket>`
      (VM service account already has objectAdmin)
- [ ] Backups verified (`./deploy/backup.sh` + cron installed)
- [ ] SSH firewall locked to your IP (already via `allowed_ssh_cidr`)

## Known follow-ups (out of scope of this plan)
- Mobile (Flutter) PayOS return needs a deep link, not the web return URL.
- React static site hosting (Cloudflare Pages / Vercel) is a separate task.
- Push notifications (FCM) not implemented.
- GitHub Actions CI/CD automation.
````

- [ ] **Step 2: Commit + push**

```
git add deploy/README.md
git commit -m "docs(deploy): runbook + production cutover checklist"
git push
```

---

## Self-Review Notes

- **Spec coverage:** VM/compose (Tasks 2–3, 6–8) ✓; Terraform 5–6 resources (Tasks 5–6) ✓; Caddy HTTPS (Task 3, 8) ✓; DuckDNS (Task 7) ✓; local→GCS storage (env in Task 3 + cutover Task 11) ✓; secrets `.env` 600 (Task 8) ✓; backup (Task 10) ✓; migrations (compose `migrate` service, Task 3/8) ✓; deploy flow (Task 9) ✓; cutover checklist incl. PayOS individual (Task 11) ✓; CORS / multi-client §12 (Task 1, verified Task 8) ✓.
- **Type consistency:** `httpmw.CORS([]string)` defined Task 1 / called Task 1 main.go; `CORSConfig.AllowedOrigins` consistent; env var `CORS_ALLOWED_ORIGINS` consistent across config, .env.example, .env.production.example, Caddy unrelated.
- **No placeholders:** all file contents are complete; `<YOUR_PROJECT_ID>`, `<api_ip>`, `<ssh_user>` are genuine per-user values, not code stubs.
