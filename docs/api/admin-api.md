# WearWhere Admin API

Tài liệu các endpoint dành cho **Admin Portal** — web app viết bằng **React JS** (actor *Platform Admin*, UC52–62 trong SRS). Mọi endpoint trả JSON, JWT-based auth.

- Server: `cmd/api` (Go + gin), module path `github.com/wearwhere/wearwhere_be`
- Frontend: Admin Portal là web app React (chạy trên trình duyệt) — KHÁC với app khách hàng (Flutter mobile). Tài liệu cho app mobile: [flutter-integration.md](./flutter-integration.md).
- Base URL dev: `http://localhost:8080/api/v1`
- Base URL demo (GCP VM): `https://34-87-41-62.sslip.io/api/v1`
- Auth: header `Authorization: Bearer <access_token>` — token phải thuộc tài khoản **role = `admin`**
- Content-Type: `application/json`

> **Phạm vi hiện tại:** mới triển khai **Manage Users — list** (`GET /admin/users`, UC54 phần 1) và **Promo codes** (`/admin/promo-codes`, đã có từ trước). Các endpoint admin khác trong SRS chưa làm — xem [§5 Roadmap](#5-roadmap-chưa-triển-khai).

---

## 1. Tích hợp từ web React

### Base URL theo môi trường

Trên web, đọc base URL từ biến môi trường build-time (Vite: `import.meta.env`, CRA/Next: `process.env`):

```js
// src/api/config.js
export const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";
```

### CORS

Admin Portal gọi API từ origin khác (vd. `http://localhost:5173` → `http://localhost:8080`), nên backend phải bật CORS cho origin của portal và cho header `Authorization`. Nếu request bị chặn bởi trình duyệt (lỗi CORS, không phải `401/403`), kiểm tra cấu hình CORS phía server thay vì sửa client.

### Axios client + gắn token

JWT trả về trong body (không phải cookie httpOnly). Lưu `access_token`/`refresh_token` ở phía client (vd. trong memory + `localStorage` cho refresh token) và gắn vào mọi request:

```js
// src/api/client.js
import axios from "axios";
import { API_BASE_URL } from "./config";

export const api = axios.create({ baseURL: API_BASE_URL });

api.interceptors.request.use((cfg) => {
  const token = sessionStorage.getItem("admin_access_token");
  if (token) cfg.headers.Authorization = `Bearer ${token}`;
  return cfg;
});

// Khi 401 → thử refresh, 403 → tài khoản không phải admin (điều hướng về trang login)
api.interceptors.response.use(
  (r) => r,
  async (err) => {
    if (err.response?.status === 401) {
      // gọi POST /auth/refresh với refresh_token, lưu lại token mới, retry
    }
    return Promise.reject(err);
  },
);
```

> **Bảo mật:** không nhúng token vào URL; chỉ gửi qua header `Authorization`. Cân nhắc lưu access token trong memory (biến/Context) để giảm rủi ro XSS, refresh token ở `localStorage`/`sessionStorage`.

---

## 2. Lấy admin token

Admin đăng nhập qua endpoint riêng (từ chối tài khoản không phải admin):

```
POST /api/v1/auth/admin/login
Content-Type: application/json

{ "email": "admin@wearwhere.vn", "password": "<password>" }
```

Trả về `AuthResponse` (giống login thường) với `user.role = "admin"`:

```json
{
  "user": { "id": "…", "email": "admin@wearwhere.vn", "role": "admin", "status": "active", "name": "Admin" },
  "tokens": {
    "access_token": "eyJ…",
    "refresh_token": "…",
    "token_type": "Bearer",
    "expires_at": "2026-07-26T10:00:00Z"
  }
}
```

Lưu `access_token` rồi gắn vào header `Authorization: Bearer <access_token>` cho mọi request dưới đây. Refresh token qua `POST /api/v1/auth/refresh` như user thường.

```js
// src/api/auth.js
import { api } from "./client";

export async function adminLogin(email, password) {
  const { data } = await api.post("/auth/admin/login", { email, password });
  sessionStorage.setItem("admin_access_token", data.tokens.access_token);
  localStorage.setItem("admin_refresh_token", data.tokens.refresh_token);
  return data.user;
}
```

---

## 3. Định dạng response & error

### Success
Trả thẳng object/`data` với HTTP `200`/`201`. (Endpoint list trả object phân trang `{ data, page, page_size, total, total_pages }`.)

### Error
Envelope chuẩn (module admin dùng `pkg/httpx`):

```json
{ "error": { "code": "FORBIDDEN", "message": "You don't have permission to perform this action" } }
```

### HTTP status liên quan tới admin auth

| Status | Khi nào | Xử lý phía portal |
|--------|---------|-------------------|
| `401 UNAUTHORIZED` | Thiếu/sai/hết hạn access token | Thử refresh; thất bại → về trang login |
| `403 FORBIDDEN` | Token hợp lệ nhưng `role != admin` | Đăng xuất / báo không đủ quyền |
| `500 INTERNAL_ERROR` | Lỗi server (vd. DB) | Hiển thị thông báo lỗi chung |

---

## 4. Endpoints

### 4.1 Manage Users — List `GET /admin/users`

Danh sách tài khoản người dùng, có tìm kiếm / sắp xếp / phân trang. Tài khoản đã xóa mềm (`status=deleted`) **không** xuất hiện. Dùng cho màn **Admin Users Page**.

**Auth:** Bearer, role = `admin`.

**Query params** (tất cả optional):

| Param | Kiểu | Mặc định | Mô tả |
|-------|------|----------|-------|
| `q` | string | — | Tìm theo `email` / `name` / `phone` (khớp một phần, không phân biệt hoa thường). Bỏ trống = lấy tất cả. |
| `sort` | enum | `created_at` | `created_at` hoặc `last_login_at`. Giá trị lạ → quay về `created_at`. |
| `order` | enum | `desc` | `asc` hoặc `desc`. Với `last_login_at`, các giá trị NULL luôn xếp cuối. Giá trị lạ → `desc`. |
| `page` | int | `1` | Trang (bắt đầu từ 1). `< 1` → `1`. |
| `page_size` | int | `20` | Số bản ghi/trang. `< 1` → `20`; tối đa `100` (vượt sẽ bị giới hạn về 100). |

**Response `200`:**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "alice@example.com",
      "phone": null,
      "name": "Alice Nguyen",
      "role": "customer",
      "status": "active",
      "email_verified": true,
      "phone_verified": false,
      "avatar_url": null,
      "last_login_at": "2026-06-20T10:00:00Z",
      "created_at": "2026-05-01T08:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 134,
  "total_pages": 7
}
```

**Trường trong mỗi item (`AdminUserResp`):**

| Field | Kiểu | Ghi chú |
|-------|------|---------|
| `id` | string (UUID) | |
| `email` | string \| null | `null` nếu user không có email (vd. đăng ký bằng phone) |
| `phone` | string \| null | |
| `name` | string | |
| `role` | string | `customer` \| `brand` \| `admin` |
| `status` | string | `active` \| `locked` (đã xóa không trả về) |
| `email_verified` | bool | suy ra từ `email_verified_at` |
| `phone_verified` | bool | suy ra từ `phone_verified_at` |
| `avatar_url` | string \| null | |
| `last_login_at` | string (RFC3339) \| null | `null` nếu chưa từng đăng nhập |
| `created_at` | string (RFC3339) | |

> Không trả về `bio` và `password_hash` (cố tình ẩn).

**Phân trang:** `total` là tổng số bản ghi khớp filter (không tính `LIMIT`); `total_pages = ceil(total / page_size)`, bằng `0` khi `total = 0`.

**Ví dụ gọi từ React (axios):**

```js
// src/api/users.js
import { api } from "./client";

export async function listUsers({ q, sort, order, page = 1, pageSize = 20 } = {}) {
  const { data } = await api.get("/admin/users", {
    params: { q, sort, order, page, page_size: pageSize },
  });
  return data; // { data, page, page_size, total, total_pages }
}
```

```js
// Hoặc dùng fetch thuần
const params = new URLSearchParams({ page: "1", page_size: "5", sort: "created_at", order: "desc" });
const res = await fetch(`${API_BASE_URL}/admin/users?${params}`, {
  headers: { Authorization: `Bearer ${token}` },
});
const body = await res.json();
```

**Lỗi:** `401` (không token), `403` (không phải admin).

---

### 4.2 Promo codes — `/admin/promo-codes`

Quản lý mã giảm giá (đã có từ trước; xem chi tiết payload trong [internal/promo](../../internal/promo)).

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/admin/promo-codes` | Tạo mã |
| GET | `/admin/promo-codes` | List (`page`, `page_size`, `active_only`) |
| GET | `/admin/promo-codes/:id` | Chi tiết |
| PATCH | `/admin/promo-codes/:id` | Cập nhật |

---

## 5. Roadmap (chưa triển khai)

Các use case admin trong SRS còn lại — sẽ làm theo từng spec/plan riêng:

| UC | Tính năng | Trạng thái |
|----|-----------|-----------|
| 52 | Login to Admin CMS | ✅ `POST /auth/admin/login` |
| 54 | Manage Users — **list** | ✅ `GET /admin/users` |
| 54 | Manage Users — detail / suspend / unsuspend / delete | ⬜ chưa làm (cần thêm admin audit log khi có mutation) |
| 53 | Verify Brand Applications | ⬜ |
| 55 | Moderate Content | ⬜ |
| 56 | Manage Product Listings | ⬜ |
| 57 | Handle Reports | ⬜ |
| 58 | Configure Platform Settings | ⬜ |
| 59 | Manage News & Editorial | ⬜ |
| 60 | View System Analytics | ⬜ |
| 61 | Generate Reports | ⬜ |
| 62 | Monitor Transactions | ⬜ |

---

## Tham chiếu

- Spec: [docs/superpowers/specs/2026-06-26-admin-manage-users-list-design.md](../superpowers/specs/2026-06-26-admin-manage-users-list-design.md)
- Plan: [docs/superpowers/plans/2026-06-26-admin-manage-users-list.md](../superpowers/plans/2026-06-26-admin-manage-users-list.md)
- Code: [internal/admin/user/](../../internal/admin/user/) · Wiring: [cmd/api/main.go](../../cmd/api/main.go)
- API app khách hàng (Flutter mobile): [docs/api/flutter-integration.md](./flutter-integration.md)
