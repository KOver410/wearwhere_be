# Deployment — WearWhere BE lên GCP (1 VM + docker-compose, Terraform IaC)

**Date:** 2026-06-04
**Branch (target):** `sprint-3-orders-checkout` (continuation) — hoặc branch riêng `deploy-gcp`
**Status:** APPROVED FOR PLANNING
**Scope:** Đưa toàn bộ backend (Go API + PostgreSQL + Redis + object storage) lên 1 VM GCP Compute Engine, chạy bằng docker-compose, provision hạ tầng bằng Terraform. Giai đoạn demo (gần như $0 nhờ free trial) dùng chung hạ tầng với production (~$15-18/tháng) để **không phải làm lại** khi go-live sau ~1 tháng.

---

## 1. Bối cảnh & ràng buộc

| Yếu tố | Giá trị |
|--------|---------|
| Mục đích | Demo đồ án bây giờ → production thật sau ~1 tháng |
| Quy mô prod dự kiến | Nhỏ (~vài trăm user, vài chục online cùng lúc) |
| Vị trí user | Việt Nam |
| Credit cloud | Chưa có (sẽ dùng GCP $300 free trial, 90 ngày) |
| Ngân sách prod | Cân bằng, $15-40/tháng |
| Tư cách pháp lý | Cá nhân, không có pháp nhân |
| Hạ tầng đã chốt | GCP Compute Engine (1 VM tự quản), region `asia-southeast1` (Singapore) |
| IaC đã chốt | Terraform, scope tối thiểu |

## 2. Key Design Decisions

| Quyết định | Lựa chọn | Lý do |
|-----------|----------|-------|
| Hình thức compute | 1× Compute Engine VM `e2-small` (2 vCPU shared, 2GB RAM), Ubuntu 22.04 LTS | Rẻ nhất trong nhóm always-on (~$13-15/tháng); chạy được cả 4 service cho quy mô vài trăm user. Nâng `e2-medium` (4GB, ~$27) nếu RAM chật |
| Cách chạy app | docker-compose (mở rộng `docker-compose.yml` có sẵn) | Demo & prod cùng môi trường; tái lập dễ; đã có sẵn Postgres+Redis trong compose |
| Provision hạ tầng | Terraform scope tối thiểu (~5 resource), state lưu trên GCS backend | Tái lập khi migrate free-trial→prod (đổi project có billing); điểm cộng capstone. Không over-engineer |
| Bootstrap Docker trên VM | Compute Engine **startup script** (Terraform truyền vào `metadata.startup-script`) | Cài Docker + clone repo tự động khi VM khởi tạo; ranh giới sạch với app layer |
| Reverse proxy + HTTPS | Caddy container (tự động Let's Encrypt) | HTTPS miễn phí, tự gia hạn cert; cần cho PayOS & OAuth |
| Domain (demo) | DuckDNS free subdomain (vd `wearwhere.duckdns.org`) | Có HTTPS thật, $0; PayOS đang `mock` nên chưa cần domain thật |
| Domain (prod) | Domain thật (~$10-15/năm) | **Không bắt buộc** với PayOS cá nhân, nhưng nên có cho niềm tin user lúc thanh toán + ổn định webhook (không phụ thuộc DuckDNS free) |
| Lưu trữ ảnh (demo) | Local disk trên VM (`STORAGE_LOCAL_DIR`), Caddy serve `/uploads` | Đơn giản, đã hỗ trợ sẵn trong code |
| Lưu trữ ảnh (prod) | GCS bucket (`STORAGE_GCS_BUCKET`) | Bền vững, ~$1-3/tháng, nằm trong GCP, **code đã hỗ trợ sẵn** — chỉ đổi env, không sửa code |
| Secrets | File `.env` trên VM, quyền `600`, không commit | Đủ cho quy mô này; GCP Secret Manager là nâng cấp prod tuỳ chọn (YAGNI hiện tại) |
| Backup DB | Cron `pg_dump` hằng ngày → GCS bucket + VM snapshot định kỳ | Bắt buộc bật trước prod; demo có thể tắt |
| Migration DB | `golang-migrate` chạy lúc deploy (format đã dùng trong `db/migrations/`) | Nhất quán với codebase |
| Deploy flow | Thủ công trước (SSH + script), GitHub Actions để sau | YAGNI; ổn định rồi mới tự động hoá |
| Cổng thanh toán | PayOS tài khoản **cá nhân** (xác thực bằng CCCD, không cần MST/giấy phép KD) | Đã xác minh: PayOS hỗ trợ cá nhân/startup chưa lập pháp nhân; gói miễn phí từ 23/01/2026 |

## 3. Kiến trúc & phân tầng

Ba tầng tách bạch, mỗi tầng một công cụ:

```
┌─ Terraform ──────────────────────────────────────────────┐
│  tạo: VM e2-small + static IP + firewall + GCS bucket     │
│       + service account (VM ghi GCS)                      │
│  state: lưu ở GCS backend (bucket riêng)                  │
└───────────────────────────┬──────────────────────────────┘
                            │ metadata.startup-script
                            ▼
┌─ startup script (chạy 1 lần khi VM tạo) ─────────────────┐
│  cài Docker + docker-compose plugin; git clone repo       │
└───────────────────────────┬──────────────────────────────┘
                            │ docker compose up -d
                            ▼
┌─ docker-compose (trên VM) ───────────────────────────────┐
│  caddy ──(80/443, auto-TLS)──► api (Go, :8080)            │
│  api ──► postgres:16  (mạng nội bộ, KHÔNG mở ra ngoài)    │
│  api ──► redis:7      (mạng nội bộ, KHÔNG mở ra ngoài)    │
│  api ──► GCS bucket (prod) hoặc local volume (demo)       │
└──────────────────────────────────────────────────────────┘
        │ webhook/callback (HTTPS)
        ▼
   PayOS · Goship · SMTP · Twilio · Google/Apple OAuth  (bên thứ 3)
```

### 3.1 Terraform resources (scope tối thiểu)

| Resource | Mục đích |
|----------|----------|
| `google_compute_instance` | VM e2-small Ubuntu 22.04, gắn startup script |
| `google_compute_address` | Static external IP (để DNS không đổi khi reboot) |
| `google_compute_firewall` | Mở 80/443 cho mọi nguồn; 22 (SSH) giới hạn theo IP của bạn |
| `google_storage_bucket` | 1 bucket cho ảnh prod + backup (có thể tách 2 bucket) |
| `google_service_account` + IAM binding | Cho VM quyền ghi GCS (Workload Identity / key) |
| (backend) `google_storage_bucket` | Bucket lưu Terraform state |

### 3.2 Containers (mở rộng `docker-compose.yml`)

| Container | Image | Trạng thái | Cổng |
|-----------|-------|-----------|------|
| `postgres` | `postgres:16-alpine` | đã có | nội bộ |
| `redis` | `redis:7-alpine` | đã có | nội bộ |
| `api` | build từ Dockerfile multi-stage (Go 1.23) | **thêm mới** | nội bộ `:8080` |
| `caddy` | `caddy:2-alpine` | **thêm mới** | `80`, `443` |

## 4. Network & bảo mật

- **Firewall GCP:** chỉ `80/443` (Caddy) mở public; `22` (SSH) giới hạn theo IP nhà/bạn. Postgres `5432` và Redis `6379` **không** có rule mở → chỉ truy cập được trong docker network.
- **HTTPS:** Caddy tự xin & gia hạn Let's Encrypt cho domain (DuckDNS demo / domain thật prod).
- **Secrets:** `.env` quyền `600`, owner non-root; không nằm trong git. `JWT_SECRET` sinh mới bằng `openssl rand -base64 64`.
- **DB/Redis:** không bind ra `0.0.0.0` host; chỉ expose trong mạng compose nội bộ.

## 5. Storage chiến lược

- **Demo:** `STORAGE_LOCAL_DIR=./uploads` (named volume), Caddy serve `/uploads`. Mất khi xoá VM → chấp nhận được ở demo.
- **Prod:** `STORAGE_GCS_BUCKET=<bucket>`, VM dùng service account để ghi. Code đã có `internal/shared/storage/gcs.go` → chỉ đổi env. URL ảnh: `https://storage.googleapis.com/<bucket>/<key>`.

## 6. Backup & phục hồi (prod)

- **Cron `pg_dump` hằng ngày** → nén → upload GCS bucket (giữ N bản gần nhất).
- **VM snapshot** định kỳ (schedule của Compute Engine) để phục hồi nhanh toàn máy.
- Tài liệu hoá thủ tục restore (pull dump từ GCS → `psql` vào container).

## 7. Quy trình deploy

**Khởi tạo lần đầu:**
1. `terraform apply` → tạo hạ tầng; startup script cài Docker + clone repo.
2. SSH vào VM, tạo `.env` thật (PayOS/Goship/SMTP/JWT...).
3. `docker compose up -d --build`.
4. Chạy migration (`golang-migrate` up).
5. Trỏ DuckDNS (demo) / DNS domain (prod) về static IP → Caddy tự cấp cert.

**Cập nhật về sau (thủ công):**
```
ssh vm → git pull → docker compose up -d --build → migrate up
```
(Đóng gói thành 1 script `deploy.sh`. GitHub Actions tự động hoá là follow-up, không làm trong scope này.)

## 8. Chuyển demo → production (checklist cutover)

| Hạng mục | Demo | Production |
|----------|------|-----------|
| `PAYOS_MODE` | `mock` | `production` + client_id/api_key/checksum thật |
| Tài khoản PayOS | — | Đăng ký cá nhân (CCCD + tài khoản ngân hàng đúng tên) |
| `GOSHIP_MODE` / `SHIPPING_PROVIDER` | `mock` / `flat` | `production`(hoặc `sandbox`) / `goship` + token |
| Storage | local disk | GCS bucket |
| Domain | DuckDNS | domain thật, đổi `PAYOS_RETURN_URL`/`CANCEL_URL`/`PAYOS_BASE_URL` |
| OAuth | trống | điền `GOOGLE_CLIENT_IDS` / `APPLE_CLIENT_IDS` |
| Backup | tắt | bật cron `pg_dump` + VM snapshot |
| SSH firewall | mở rộng | khoá theo IP |
| `JWT_SECRET` | demo | sinh mới, mạnh |
| `APP_ENV` | `development` | `production` |

## 9. Chi phí

| Giai đoạn | Chi phí/tháng |
|-----------|---------------|
| Demo (trong 90 ngày free trial $300) | ~$0 (trừ vào credit) |
| Prod sau free trial | VM e2-small ~$13-15 + GCS ~$1-3 = **~$15-18** |
| Nếu nâng e2-medium (4GB) khi traffic tăng | ~$27 + GCS = ~$28-30 |
| PayOS phí cổng | ~$0 (gói miễn phí cá nhân từ 23/01/2026) |
| Domain (prod, tuỳ chọn) | ~$10-15/năm |

**So sánh đã loại bỏ:** GCP managed đầy đủ (Cloud Run + Cloud SQL + Memorystore) ~$63-70/tháng — vượt ngân sách, chủ yếu do Memorystore Redis tối thiểu ~$35/tháng không có gói rẻ.

## 10. Non-goals (ngoài scope spec này)

- GitHub Actions CI/CD tự động (follow-up sau khi deploy thủ công ổn định).
- GCP Secret Manager (dùng `.env` là đủ cho quy mô này).
- Auto-scaling / load balancer / multi-instance (quy mô nhỏ, 1 VM đủ).
- Managed Postgres/Redis (Cloud SQL/Memorystore) — đã loại vì chi phí.
- Monitoring/alerting nâng cao (có thể thêm `docker logs` + GCP Ops Agent cơ bản sau).
- Multi-region / CDN cho ảnh (GCS public URL đủ cho giai đoạn đầu).
- **Host React web (static site)** — theo yêu cầu, chưa deploy web JS đợt này. BE chỉ phục vụ API; React deploy ở task riêng sau (Cloudflare Pages/Vercel/Netlify — static, gần như $0).
- **Push notification (FCM/APNS)** cho Flutter — là feature mới, không thuộc hạ tầng; quyết định riêng nếu cần.

## 11. Rủi ro & giảm thiểu

| Rủi ro | Giảm thiểu |
|--------|-----------|
| Free trial hết → phải đổi project có billing | Terraform `apply` lên project mới dựng lại nhanh; backup DB sẵn trên GCS |
| RAM 2GB chật khi Postgres+Redis+Go cùng chạy | Theo dõi; nâng e2-medium 1 dòng lệnh `terraform apply` |
| DuckDNS down → webhook PayOS lỗi | Demo dùng `mock` nên không ảnh hưởng tiền; prod chuyển domain thật |
| Mất data trên local disk khi xoá VM | Demo chấp nhận; prod dùng GCS + backup |
| SSH lộ ra internet | Firewall giới hạn IP; cân nhắc IAP/OS Login |
| React web bị browser chặn vì thiếu CORS | Thêm CORS middleware (§12) — coi là bước bắt buộc trong plan deploy |

## 12. Multi-client (Flutter app + React web)

BE phục vụ **2 client** dùng chung một API HTTPS: **Flutter app** (native iOS/Android) và **React web** (browser). Hạ tầng (VM/network/storage) **không đổi** — mọi client gọi cùng endpoint. Nhưng có các điểm cross-cutting bắt buộc xử lý:

| # | Vấn đề | Trạng thái | Hành động |
|---|--------|-----------|-----------|
| 1 | **CORS** — React (browser) gọi API sẽ bị chặn nếu thiếu `Access-Control-Allow-Origin`. Flutter native KHÔNG bị (CORS là cơ chế browser). | 🔴 **THIẾU** trong code (không có middleware/dependency) | **Bước bắt buộc trong plan deploy:** thêm `gin-contrib/cors`, cấu hình allowed origins từ env `CORS_ALLOWED_ORIGINS` (origin React dev + prod). Bật allow credentials nếu dùng cookie; ở đây dùng Bearer token nên chủ yếu cần allow `Authorization` header. |
| 2 | **PayOS return/cancel URL** cứng cho web (`localhost:3000/...`) | 🟡 Web OK, mobile chưa | Web giữ `PAYOS_RETURN_URL` web. Mobile cần deep link (vd `wearwhere://checkout/success`) hoặc in-app webview redirect. Xử lý trước khi mobile bật PayOS prod (ngoài scope deploy này, ghi nhận để app team xử lý). |
| 3 | **OAuth multi-platform** (`GOOGLE_CLIENT_IDS`/`APPLE_CLIENT_IDS` nhiều giá trị: web + iOS + Android) | 🟢 Code đã hỗ trợ | Chỉ cần điền đúng client IDs theo từng nền tảng khi cutover. |
| 4 | **HTTPS bắt buộc** (iOS ATS + browser) | 🟢 Caddy auto-TLS đã có | Không cần thêm. |
| 5 | **Push notification** (Flutter) | ⚪ Chưa có | Out of scope — feature mới, quyết định riêng (FCM). |

**Gộp vào plan deploy:** mục #1 (CORS middleware + env) là một task bắt buộc trong kế hoạch triển khai, hoàn thành trước khi React web kết nối.
