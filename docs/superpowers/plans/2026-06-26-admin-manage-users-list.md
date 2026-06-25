# Admin Manage Users (List) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `GET /api/v1/admin/users` — a paginated, searchable, sortable list of user accounts for platform admins (UC54, part 1).

**Architecture:** New self-contained module `internal/admin/user` (domain / repo / service / handler / routes), mirroring the existing `internal/promo` module. The repo queries the `users` table directly via the shared `*pgxpool.Pool`; the module does not import the `auth` domain. The route mounts on the existing admin group in `cmd/api/main.go`, which already enforces `RequireAuth` + `RequireRole(RoleAdmin)`.

**Tech Stack:** Go 1.25, gin, pgx/v5 (pgxpool), testify, project `pkg/httpx` helpers.

## Global Constraints

- Module path: `github.com/wearwhere/wearwhere_be`.
- Soft-deleted users are excluded: every query filters `deleted_at IS NULL`.
- Read-only feature: no mutations, no admin audit log in this plan.
- `sort`/`order` are never string-interpolated from raw input — only whitelisted enum values map to fixed `ORDER BY` fragments.
- Sort whitelist: `created_at` (default), `last_login_at`. Order: `asc`, `desc` (default `desc`). `last_login_at` uses `NULLS LAST`.
- Pagination: `page` default 1 (min 1); `page_size` default 20, max 100.
- HTTP responses use `pkg/httpx` (`httpx.OK`, `httpx.ErrorFromApp`).
- Commit messages: no `Co-Authored-By` trailer.
- Unit tests: `go test ./... -race`. Integration tests (need DB): `make test-integration` (build tag `integration`, requires `TEST_DATABASE_URL`).

## File Structure

- `internal/admin/user/domain/dto.go` — `ListUsersFilter` (+ `Normalized()`), `AdminUserRow`, `AdminUserResp`, `AdminUserListResp`, `ToResp`, sort/order constants.
- `internal/admin/user/domain/dto_test.go` — unit tests for `Normalized()` and `ToResp`.
- `internal/admin/user/repo/repo.go` — `ReadRepo` interface.
- `internal/admin/user/repo/user_read_pg.go` — `UserReadPG` implementing `ReadRepo` against Postgres.
- `internal/admin/user/repo/user_read_pg_test.go` — integration test (build tag `integration`).
- `internal/admin/user/service/service.go` — `Service.ListUsers`.
- `internal/admin/user/service/service_test.go` — unit test with in-memory fake repo.
- `internal/admin/user/handler/handler.go` — `Handler.List`.
- `internal/admin/user/handler/routes.go` — `MountAdmin`.
- `internal/admin/user/handler/handler_test.go` — handler test (real service + fake repo).
- `cmd/api/main.go` — wire the module into the existing `adminGroup` (modify).

---

### Task 1: Domain DTOs, filter normalization, and mapping

**Files:**
- Create: `internal/admin/user/domain/dto.go`
- Test: `internal/admin/user/domain/dto_test.go`

**Interfaces:**
- Consumes: nothing (leaf).
- Produces:
  - Constants: `SortCreatedAt = "created_at"`, `SortLastLogin = "last_login_at"`, `OrderAsc = "asc"`, `OrderDesc = "desc"`.
  - `type ListUsersFilter struct { Q string; Sort string; Order string; Page int; PageSize int }` with method `func (f ListUsersFilter) Normalized() ListUsersFilter`.
  - `type AdminUserRow struct { ID uuid.UUID; Email *string; Phone *string; Name string; Role string; Status string; EmailVerifiedAt *time.Time; PhoneVerifiedAt *time.Time; AvatarURL *string; LastLoginAt *time.Time; CreatedAt time.Time }`.
  - `type AdminUserResp struct {...}` (JSON wire form).
  - `type AdminUserListResp struct { Data []AdminUserResp; Page int; PageSize int; Total int; TotalPages int }`.
  - `func ToResp(r AdminUserRow) AdminUserResp`.

- [ ] **Step 1: Write the failing test**

Create `internal/admin/user/domain/dto_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

func TestNormalized_Defaults(t *testing.T) {
	got := domain.ListUsersFilter{}.Normalized()
	assert.Equal(t, domain.SortCreatedAt, got.Sort)
	assert.Equal(t, domain.OrderDesc, got.Order)
	assert.Equal(t, 1, got.Page)
	assert.Equal(t, 20, got.PageSize)
	assert.Equal(t, "", got.Q)
}

func TestNormalized_ClampsAndFallbacks(t *testing.T) {
	got := domain.ListUsersFilter{
		Q: "  alice  ", Sort: "bogus", Order: "sideways", Page: -3, PageSize: 500,
	}.Normalized()
	assert.Equal(t, "alice", got.Q)             // trimmed
	assert.Equal(t, domain.SortCreatedAt, got.Sort)  // unknown -> default
	assert.Equal(t, domain.OrderDesc, got.Order)     // unknown -> default
	assert.Equal(t, 1, got.Page)                // <1 -> 1
	assert.Equal(t, 100, got.PageSize)          // >100 -> 100
}

func TestNormalized_KeepsValidValues(t *testing.T) {
	got := domain.ListUsersFilter{
		Sort: domain.SortLastLogin, Order: domain.OrderAsc, Page: 3, PageSize: 50,
	}.Normalized()
	assert.Equal(t, domain.SortLastLogin, got.Sort)
	assert.Equal(t, domain.OrderAsc, got.Order)
	assert.Equal(t, 3, got.Page)
	assert.Equal(t, 50, got.PageSize)
}

func TestNormalized_ZeroPageSizeDefaults(t *testing.T) {
	assert.Equal(t, 20, domain.ListUsersFilter{PageSize: 0}.Normalized().PageSize)
}

func TestToResp_MapsAndDerivesVerifiedFlags(t *testing.T) {
	now := time.Now()
	email := "a@b.com"
	id := uuid.New()
	row := domain.AdminUserRow{
		ID: id, Email: &email, Phone: nil, Name: "Alice",
		Role: "customer", Status: "active",
		EmailVerifiedAt: &now, PhoneVerifiedAt: nil,
		CreatedAt: now,
	}
	resp := domain.ToResp(row)
	assert.Equal(t, id.String(), resp.ID)
	assert.Equal(t, &email, resp.Email)
	assert.Equal(t, "Alice", resp.Name)
	assert.True(t, resp.EmailVerified)
	assert.False(t, resp.PhoneVerified)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/user/domain/ -run Normalized -v`
Expected: FAIL — package/types do not exist (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/admin/user/domain/dto.go`:

```go
// Package domain holds the read DTOs and query filter for the admin
// user-listing endpoint (UC54 — list).
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100

	SortCreatedAt = "created_at"
	SortLastLogin = "last_login_at"
	OrderAsc      = "asc"
	OrderDesc     = "desc"
)

// ListUsersFilter holds the query parameters for GET /admin/users.
type ListUsersFilter struct {
	Q        string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// Normalized returns a copy with defaults applied and values clamped to the
// allowed ranges/whitelists, safe to hand to the repo.
func (f ListUsersFilter) Normalized() ListUsersFilter {
	n := f
	n.Q = strings.TrimSpace(n.Q)
	switch n.Sort {
	case SortCreatedAt, SortLastLogin:
	default:
		n.Sort = SortCreatedAt
	}
	switch n.Order {
	case OrderAsc, OrderDesc:
	default:
		n.Order = OrderDesc
	}
	if n.Page < 1 {
		n.Page = 1
	}
	if n.PageSize < 1 {
		n.PageSize = defaultPageSize
	}
	if n.PageSize > maxPageSize {
		n.PageSize = maxPageSize
	}
	return n
}

// AdminUserRow is the repo-level read model scanned from the users table.
type AdminUserRow struct {
	ID              uuid.UUID
	Email           *string
	Phone           *string
	Name            string
	Role            string
	Status          string
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	AvatarURL       *string
	LastLoginAt     *time.Time
	CreatedAt       time.Time
}

// AdminUserResp is the wire representation of a user in the admin list.
type AdminUserResp struct {
	ID            string     `json:"id"`
	Email         *string    `json:"email"`
	Phone         *string    `json:"phone"`
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	EmailVerified bool       `json:"email_verified"`
	PhoneVerified bool       `json:"phone_verified"`
	AvatarURL     *string    `json:"avatar_url"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AdminUserListResp is a paginated list of users.
type AdminUserListResp struct {
	Data       []AdminUserResp `json:"data"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

// ToResp maps a repo row to its wire DTO.
func ToResp(r AdminUserRow) AdminUserResp {
	return AdminUserResp{
		ID:            r.ID.String(),
		Email:         r.Email,
		Phone:         r.Phone,
		Name:          r.Name,
		Role:          r.Role,
		Status:        r.Status,
		EmailVerified: r.EmailVerifiedAt != nil,
		PhoneVerified: r.PhoneVerifiedAt != nil,
		AvatarURL:     r.AvatarURL,
		LastLoginAt:   r.LastLoginAt,
		CreatedAt:     r.CreatedAt,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/admin/user/domain/ -v`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/user/domain/
git commit -m "feat(admin): add admin user list domain DTOs + filter normalization"
```

---

### Task 2: ReadRepo interface + service

**Files:**
- Create: `internal/admin/user/repo/repo.go`
- Create: `internal/admin/user/service/service.go`
- Test: `internal/admin/user/service/service_test.go`

**Interfaces:**
- Consumes: `domain.ListUsersFilter`, `domain.AdminUserRow`, `domain.AdminUserListResp`, `domain.ToResp` (Task 1).
- Produces:
  - `repo.ReadRepo` interface: `ListUsers(ctx context.Context, f domain.ListUsersFilter) (items []domain.AdminUserRow, total int, err error)`.
  - `service.New(r repo.ReadRepo) *service.Service`.
  - `func (s *Service) ListUsers(ctx context.Context, raw domain.ListUsersFilter) (domain.AdminUserListResp, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/admin/user/service/service_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
)

// fakeReadRepo records the filter it was called with and returns canned data.
type fakeReadRepo struct {
	gotFilter domain.ListUsersFilter
	rows      []domain.AdminUserRow
	total     int
	err       error
}

func (f *fakeReadRepo) ListUsers(_ context.Context, flt domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	f.gotFilter = flt
	return f.rows, f.total, f.err
}

func TestListUsers_NormalizesFilterBeforeRepo(t *testing.T) {
	fake := &fakeReadRepo{}
	svc := service.New(fake)
	_, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{PageSize: 999, Sort: "x"})
	require.NoError(t, err)
	assert.Equal(t, 100, fake.gotFilter.PageSize)        // clamped
	assert.Equal(t, domain.SortCreatedAt, fake.gotFilter.Sort) // fallback
	assert.Equal(t, 1, fake.gotFilter.Page)              // default
}

func TestListUsers_MapsRowsAndPagination(t *testing.T) {
	fake := &fakeReadRepo{
		rows:  []domain.AdminUserRow{{ID: uuid.New(), Name: "A"}, {ID: uuid.New(), Name: "B"}},
		total: 134,
	}
	svc := service.New(fake)
	resp, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "A", resp.Data[0].Name)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
	assert.Equal(t, 134, resp.Total)
	assert.Equal(t, 7, resp.TotalPages) // ceil(134/20)
}

func TestListUsers_EmptyResult(t *testing.T) {
	svc := service.New(&fakeReadRepo{rows: nil, total: 0})
	resp, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{})
	require.NoError(t, err)
	assert.NotNil(t, resp.Data) // non-nil empty slice -> serializes as []
	assert.Len(t, resp.Data, 0)
	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 0, resp.TotalPages)
}

func TestListUsers_RepoErrorPropagates(t *testing.T) {
	svc := service.New(&fakeReadRepo{err: errors.New("db down")})
	_, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/user/service/ -v`
Expected: FAIL — `repo` and `service` packages do not exist (compile error).

- [ ] **Step 3a: Write the repo interface**

Create `internal/admin/user/repo/repo.go`:

```go
// Package repo defines the read-only persistence port for admin user listing.
package repo

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

// ReadRepo loads users for the admin list endpoint.
type ReadRepo interface {
	ListUsers(ctx context.Context, f domain.ListUsersFilter) (items []domain.AdminUserRow, total int, err error)
}
```

- [ ] **Step 3b: Write the service**

Create `internal/admin/user/service/service.go`:

```go
// Package service implements the admin user-listing use case (UC54 — list).
package service

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/repo"
)

type Service struct{ repo repo.ReadRepo }

func New(r repo.ReadRepo) *Service { return &Service{repo: r} }

// ListUsers normalizes the filter, queries the repo, and maps rows to the
// paginated wire response.
func (s *Service) ListUsers(ctx context.Context, raw domain.ListUsersFilter) (domain.AdminUserListResp, error) {
	f := raw.Normalized()
	rows, total, err := s.repo.ListUsers(ctx, f)
	if err != nil {
		return domain.AdminUserListResp{}, err
	}
	data := make([]domain.AdminUserResp, 0, len(rows))
	for _, r := range rows {
		data = append(data, domain.ToResp(r))
	}
	resp := domain.AdminUserListResp{
		Data:     data,
		Page:     f.Page,
		PageSize: f.PageSize,
		Total:    total,
	}
	if f.PageSize > 0 {
		resp.TotalPages = (total + f.PageSize - 1) / f.PageSize
	}
	return resp, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/admin/user/service/ -v`
Expected: PASS (all 4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/user/repo/repo.go internal/admin/user/service/
git commit -m "feat(admin): add admin user list service + read repo port"
```

---

### Task 3: Postgres read repo (`UserReadPG.ListUsers`)

**Files:**
- Create: `internal/admin/user/repo/user_read_pg.go`
- Test: `internal/admin/user/repo/user_read_pg_test.go` (integration, build tag `integration`)

**Interfaces:**
- Consumes: `domain.ListUsersFilter`, `domain.AdminUserRow`, sort/order constants (Task 1); `repo.ReadRepo` (Task 2 — `UserReadPG` must satisfy it).
- Produces: `func NewUserReadPG(db *pgxpool.Pool) *UserReadPG` and its `ListUsers` method (satisfies `ReadRepo`).

- [ ] **Step 1: Write the failing test**

Create `internal/admin/user/repo/user_read_pg_test.go`:

```go
//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		panic("TEST_DATABASE_URL not set; run via `make test-integration`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func TestListUsers_PaginatesAndCountsTotal(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	for i := 0; i < 3; i++ {
		testfixtures.SeedUser(t, testPool, "customer")
	}

	items, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)   // COUNT(*) OVER() ignores LIMIT
	assert.Len(t, items, 2)     // page capped to 2
}

func TestListUsers_ExcludesDeleted(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	u := testfixtures.SeedUser(t, testPool, "customer")
	_, err := testPool.Exec(ctx,
		`UPDATE users SET status='deleted', deleted_at=NOW() WHERE id=$1`, u.ID)
	require.NoError(t, err)

	_, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestListUsers_SearchByName(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	testfixtures.SeedUser(t, testPool, "customer") // name "Test customer"
	testfixtures.SeedUser(t, testPool, "brand")    // name "Test brand"

	items, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Q: "brand", Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "brand", items[0].Role)
}

func TestListUsers_EmptyTableReturnsZero(t *testing.T) {
	testfixtures.Clean(t, testPool)
	r := NewUserReadPG(testPool)
	items, total, err := r.ListUsers(context.Background(), domain.ListUsersFilter{
		Sort: domain.SortLastLogin, Order: domain.OrderAsc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Len(t, items, 0)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `make test-db-up && TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5433/wearwhere_test?sslmode=disable" go test -tags=integration ./internal/admin/user/repo/ -v`
(Use the same `TEST_DATABASE_URL` your project's `make test-integration` uses — see Makefile `TEST_DB_URL`.)
Expected: FAIL — `NewUserReadPG` undefined (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/admin/user/repo/user_read_pg.go`:

```go
package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

type UserReadPG struct{ db *pgxpool.Pool }

func NewUserReadPG(db *pgxpool.Pool) *UserReadPG { return &UserReadPG{db: db} }

// orderClauses maps validated (sort, order) enum pairs to fixed ORDER BY
// fragments. Raw user input is never interpolated into SQL.
var orderClauses = map[string]map[string]string{
	domain.SortCreatedAt: {
		domain.OrderAsc:  "created_at ASC",
		domain.OrderDesc: "created_at DESC",
	},
	domain.SortLastLogin: {
		domain.OrderAsc:  "last_login_at ASC NULLS LAST",
		domain.OrderDesc: "last_login_at DESC NULLS LAST",
	},
}

func (r *UserReadPG) ListUsers(ctx context.Context, f domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	orderBy := orderClauses[f.Sort][f.Order]
	if orderBy == "" { // defensive: f is expected pre-normalized
		orderBy = "created_at DESC"
	}

	q := `SELECT id, email, phone, name, role, status,
	             email_verified_at, phone_verified_at, avatar_url, last_login_at, created_at,
	             COUNT(*) OVER() AS total
	      FROM users
	      WHERE deleted_at IS NULL
	        AND ($1 = '' OR email ILIKE '%'||$1||'%'
	                     OR name  ILIKE '%'||$1||'%'
	                     OR phone ILIKE '%'||$1||'%')
	      ORDER BY ` + orderBy + `
	      LIMIT $2 OFFSET $3`

	offset := (f.Page - 1) * f.PageSize
	rows, err := r.db.Query(ctx, q, f.Q, f.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []domain.AdminUserRow
	total := 0
	for rows.Next() {
		var u domain.AdminUserRow
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role, &u.Status,
			&u.EmailVerifiedAt, &u.PhoneVerifiedAt, &u.AvatarURL, &u.LastLoginAt, &u.CreatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, u)
	}
	return items, total, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `TEST_DATABASE_URL="<your test DB url>" go test -tags=integration ./internal/admin/user/repo/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/user/repo/user_read_pg.go internal/admin/user/repo/user_read_pg_test.go
git commit -m "feat(admin): implement Postgres read repo for admin user list"
```

---

### Task 4: HTTP handler + routes

**Files:**
- Create: `internal/admin/user/handler/handler.go`
- Create: `internal/admin/user/handler/routes.go`
- Test: `internal/admin/user/handler/handler_test.go`

**Interfaces:**
- Consumes: `service.New`, `service.Service.ListUsers` (Task 2); `domain.ListUsersFilter` (Task 1); `repo.ReadRepo` (Task 2, for the test fake); `pkg/httpx`.
- Produces:
  - `handler.New(s *service.Service) *Handler`.
  - `func (h *Handler) List(c *gin.Context)`.
  - `func MountAdmin(rg *gin.RouterGroup, h *Handler)` registering `GET /users`.

- [ ] **Step 1: Write the failing test**

Create `internal/admin/user/handler/handler_test.go`:

```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/handler"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
)

type fakeReadRepo struct {
	gotFilter domain.ListUsersFilter
	rows      []domain.AdminUserRow
	total     int
}

func (f *fakeReadRepo) ListUsers(_ context.Context, flt domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	f.gotFilter = flt
	return f.rows, f.total, nil
}

func setup(fake *fakeReadRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(service.New(fake))
	handler.MountAdmin(r.Group("/admin"), h)
	return r
}

func TestList_Returns200WithData(t *testing.T) {
	fake := &fakeReadRepo{rows: []domain.AdminUserRow{{ID: uuid.New(), Name: "Alice"}}, total: 1}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body domain.AdminUserListResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 1, body.Total)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "Alice", body.Data[0].Name)
}

func TestList_ParsesQueryParamsIntoFilter(t *testing.T) {
	fake := &fakeReadRepo{}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/admin/users?q=bob&sort=last_login_at&order=asc&page=2&page_size=500", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// Service normalizes before the repo sees it: page_size capped to 100.
	assert.Equal(t, "bob", fake.gotFilter.Q)
	assert.Equal(t, domain.SortLastLogin, fake.gotFilter.Sort)
	assert.Equal(t, domain.OrderAsc, fake.gotFilter.Order)
	assert.Equal(t, 2, fake.gotFilter.Page)
	assert.Equal(t, 100, fake.gotFilter.PageSize)
}

func TestList_EmptyDefaults(t *testing.T) {
	fake := &fakeReadRepo{}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, domain.SortCreatedAt, fake.gotFilter.Sort)
	assert.Equal(t, domain.OrderDesc, fake.gotFilter.Order)
	assert.Equal(t, 1, fake.gotFilter.Page)
	assert.Equal(t, 20, fake.gotFilter.PageSize)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/user/handler/ -v`
Expected: FAIL — `handler` package does not exist (compile error).

- [ ] **Step 3a: Write the handler**

Create `internal/admin/user/handler/handler.go`:

```go
// Package handler exposes the admin HTTP endpoint for listing users.
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

// List handles GET /admin/users?q=&sort=&order=&page=&page_size=.
// Out-of-range / unknown values are normalized by the service.
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	f := domain.ListUsersFilter{
		Q:        c.Query("q"),
		Sort:     c.Query("sort"),
		Order:    c.Query("order"),
		Page:     page,
		PageSize: pageSize,
	}
	resp, err := h.svc.ListUsers(c.Request.Context(), f)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
```

- [ ] **Step 3b: Write the routes**

Create `internal/admin/user/handler/routes.go`:

```go
package handler

import "github.com/gin-gonic/gin"

// MountAdmin registers admin user-management routes. The caller chains admin
// auth (RequireAuth + RequireRole(admin)) onto rg.
func MountAdmin(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/users")
	g.GET("", h.List)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/admin/user/handler/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/user/handler/
git commit -m "feat(admin): add GET /admin/users handler + route"
```

---

### Task 5: Wire the module into the API

**Files:**
- Modify: `cmd/api/main.go` (import block near line 51; admin group near line 425)

**Interfaces:**
- Consumes: `repo.NewUserReadPG`, `service.New`, `handler.New`, `handler.MountAdmin` (Tasks 2–4); existing `pgPool` (`cmd/api/main.go:106`) and `adminGroup` (`cmd/api/main.go:421-424`).
- Produces: a live route `GET /api/v1/admin/users`.

- [ ] **Step 1: Add imports**

In the import block of `cmd/api/main.go` (alongside the existing `promorepo`/`promohandler` aliases near line 51), add:

```go
	adminuserhandler "github.com/wearwhere/wearwhere_be/internal/admin/user/handler"
	adminuserrepo "github.com/wearwhere/wearwhere_be/internal/admin/user/repo"
	adminuserservice "github.com/wearwhere/wearwhere_be/internal/admin/user/service"
```

- [ ] **Step 2: Mount the route on the existing admin group**

In `cmd/api/main.go`, immediately after the existing line (≈425):

```go
	promohandler.MountAdmin(adminGroup, promohandler.New(promoSvc))
```

add:

```go
	adminUserSvc := adminuserservice.New(adminuserrepo.NewUserReadPG(pgPool))
	adminuserhandler.MountAdmin(adminGroup, adminuserhandler.New(adminUserSvc))
```

- [ ] **Step 3: Build to verify wiring compiles**

Run: `go build ./...`
Expected: no output (exit 0).

- [ ] **Step 4: Run the full unit suite**

Run: `go test ./internal/admin/... -race`
Expected: PASS for `domain`, `service`, `handler` (the `repo` integration test is skipped without the `integration` tag).

- [ ] **Step 5: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(admin): wire GET /admin/users into API router"
```

---

## Manual verification (after Task 5)

1. Start the stack and obtain an admin token via `POST /api/v1/auth/admin/login`.
2. `GET /api/v1/admin/users?page=1&page_size=5&sort=created_at&order=desc` with `Authorization: Bearer <admin token>` → 200, paginated body.
3. Same call with a customer token → `403 FORBIDDEN`. No token → `401 UNAUTHORIZED`.
4. `GET /api/v1/admin/users?q=<substring>` → results filtered by email/name/phone.

## Self-Review notes

- **Spec coverage:** endpoint + params (Tasks 1,4); search `q` (Tasks 1,3); sort/order whitelist incl. NULLS LAST (Tasks 1,3); pagination defaults/caps (Task 1); response shape + `total_pages` (Tasks 1,2); excludes deleted (Task 3); module structure (all tasks); auth via existing group (Task 5); tests at each layer (every task). No gaps.
- **Type consistency:** `ListUsersFilter`, `AdminUserRow`, `AdminUserResp`, `AdminUserListResp`, `ToResp`, `ReadRepo.ListUsers`, `service.New`/`ListUsers`, `handler.New`/`List`/`MountAdmin`, `NewUserReadPG` are named identically across all tasks and the wiring. `Normalized()` defined in Task 1 is used by the service in Task 2.
- **No placeholders:** every code/test step shows complete code.
