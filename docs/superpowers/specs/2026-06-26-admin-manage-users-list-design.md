# Admin — Manage Users (List) — Design

**Date:** 2026-06-26
**UC:** UC54 *Manage Users* (phần 1 — listing). SRS: "Admins can view, suspend, or delete user accounts."
**Scope of this spec:** the read-only **list** endpoint only. Suspend / unsuspend / delete and the
deleted-accounts list are explicitly out of scope and will be separate specs.

## Goal

Give platform admins a paginated, searchable, sortable list of user accounts via
`GET /api/v1/admin/users`. This backs the "Admin Users Page" screen in the SRS.

## Non-goals (this spec)

- Mutations (suspend/unsuspend/delete) — no write paths, therefore **no admin audit log** introduced here.
- `GET /admin/users/:id` user detail.
- `GET /admin/deleted-accounts`.
- Filtering by `role` / `status` (not requested; trivial to add later — see Future Extensions).

## Architecture

New self-contained module `internal/admin/user`, mirroring the existing `internal/promo` module
style (domain / repo / service / handler / routes). It queries the `users` table directly via the
shared `*pgxpool.Pool` and maps to its own read DTO, so it does **not** import the `auth` domain.

```
internal/admin/user/
  domain/dto.go        ListUsersFilter, AdminUserRow, AdminUserResp, AdminUserListResp, ToResp
  repo/repo.go         ReadRepo interface + UserReadPG (SELECT from users)
  service/service.go   Service.ListUsers — normalize filter, compute total_pages
  handler/handler.go   Handler.List — parse query params, call service
  handler/routes.go    MountAdmin(rg, h) → rg.GET("/users", h.List)
```

Wiring in `cmd/api/main.go` reuses the existing admin group (already enforces
`RequireAuth` + `RequireRole(RoleAdmin)`):

```go
adminUserSvc := adminuserservice.New(adminuserrepo.NewUserReadPG(pgPool))
adminuserhandler.MountAdmin(adminGroup, adminuserhandler.New(adminUserSvc))
```

## API Contract

### Request

`GET /api/v1/admin/users?q=&sort=&order=&page=&page_size=`

Requires a valid admin access token (handled by the mounted middleware → 401 if unauthenticated,
403 if not an admin).

| Param        | Type   | Default      | Rules |
|--------------|--------|--------------|-------|
| `q`          | string | `""`         | Trimmed; if non-empty, matches `email` OR `name` OR `phone` via `ILIKE '%q%'`. |
| `sort`       | enum   | `created_at` | Whitelist: `created_at`, `last_login_at`. Unknown value → fallback to `created_at`. |
| `order`      | enum   | `desc`       | `asc` \| `desc`. Unknown → `desc`. For `last_login_at`, NULLs always sort last. |
| `page`       | int    | `1`          | `< 1` → `1`. |
| `page_size`  | int    | `20`         | `< 1` → `20`; capped at `100`. |

### Response `200 OK`

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "a@b.com",
      "phone": null,
      "name": "Nguyen Van A",
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

Empty result → `data: []`, `total: 0`, `total_pages: 0`. Uses `httpx.OK`.

### Errors

- `401 UNAUTHORIZED` / `403 FORBIDDEN` — from the admin middleware chain.
- No request-body validation errors (all params optional with safe fallbacks).
- `500 INTERNAL_ERROR` via `httpx.ErrorFromApp` on DB failure.

## Data Model

Reads only. Maps the `users` table (excluding soft-deleted rows: `deleted_at IS NULL`).

`AdminUserRow` (repo-level scan target) → `AdminUserResp` (wire). Fields surfaced:
`id, email, phone, name, role, status, email_verified (email_verified_at != NULL),
phone_verified (phone_verified_at != NULL), avatar_url, last_login_at, created_at`.
`bio` and `password_hash` are intentionally **not** exposed.

## Repository

```go
type ListUsersFilter struct {
    Q        string // empty = no search
    Sort     string // validated enum: "created_at" | "last_login_at"
    Order    string // validated enum: "asc" | "desc"
    Page     int    // >= 1
    PageSize int    // 1..100
}

type ReadRepo interface {
    ListUsers(ctx context.Context, f ListUsersFilter) (items []AdminUserRow, total int, err error)
}
```

`UserReadPG.ListUsers` issues a single query using `COUNT(*) OVER() AS total` so the page rows and
the grand total come back together. The `q` value is passed as a bind parameter
(`($1 = '' OR email ILIKE '%'||$1||'%' OR name ILIKE '%'||$1||'%' OR phone ILIKE '%'||$1||'%')`).
`sort` and `order` are **not** interpolated from raw input — the service hands the repo only
whitelisted enum values, and the repo maps them to fixed `ORDER BY` clause fragments
(`last_login_at` always `NULLS LAST`). `LIMIT $page_size OFFSET (page-1)*page_size`.

When the page is empty, `total` defaults to `0` (no rows → COUNT window yields nothing, handled in scan loop).

## Service

`Service.ListUsers(ctx, raw ListUsersFilter) (AdminUserListResp, error)`:

1. Normalize the filter — apply defaults/caps for `page`, `page_size`; coerce `sort`/`order` to the
   whitelist; trim `q`.
2. Call `repo.ListUsers`.
3. Map rows → `[]AdminUserResp` (non-nil slice).
4. Compute `total_pages = ceil(total / page_size)` (0 when `total == 0`).

Normalization lives in the service so the handler stays a thin adapter and the rules are unit-testable
without HTTP.

## Handler

`Handler.List(c *gin.Context)`:

- Parse `q`, `sort`, `order` (strings) and `page`, `page_size` (via `strconv.Atoi`, ignore parse error → 0 → service defaults).
- Build raw `ListUsersFilter`, call `svc.ListUsers`.
- On error → `httpx.ErrorFromApp`; on success → `httpx.OK(c, resp)`.

`MountAdmin(rg *gin.RouterGroup, h *Handler)` registers `rg.GET("/users", h.List)`. Caller chains
admin auth onto `rg` (consistent with `promo.MountAdmin`).

## Testing

- **Handler test** (mirror `internal/promo/handler/handler_test.go`): mount on a test router, assert
  route exists and returns 200 with a stub service; verify `page_size` over 100 is capped and an
  unknown `sort` falls back — assertions done against the normalized filter captured by a fake service/repo.
- **Service test** with an in-memory fake `ReadRepo`:
  - defaults: empty filter → `page=1, page_size=20, sort=created_at, order=desc`.
  - `page_size=500` → capped to `100`; `page_size=0` → `20`; `page=0` → `1`.
  - unknown `sort`/`order` → fallback values.
  - `total_pages` math: `total=134, page_size=20 → 7`; `total=0 → 0`.
  - rows mapped correctly incl. `email_verified`/`phone_verified` derivation and `data: []` on empty.

## Future Extensions (not in this spec)

- `role` / `status` filters (add fields to `ListUsersFilter` + WHERE clauses).
- Include soft-deleted users via an explicit `status=deleted` filter.
- The remaining UC54 mutations (suspend/unsuspend/delete) + admin audit log + deleted-accounts list.
