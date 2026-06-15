# Moderation (UC40 Block User) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a customer block another user so the blocked user's OOTD content disappears from the blocker's feeds, by-user listings, comment threads, and post detail.

**Architecture:** A new `block` module (`internal/block/{domain,repo,service,handler}`) mirrors the existing `follow` module: a `user_blocks` join table, idempotent block/unblock, and a list endpoint. The "hide their content from me" effect is applied inside the OOTD repository's read queries via a `NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id = $viewer)` subquery (guest viewer = `uuid.Nil` → empty subquery → no filtering). Post detail uses a dedicated `IsBlocked` lookup so internal ownership/like checks stay unfiltered.

**Tech Stack:** Go, gin, pgx/v5, golang-migrate, testify. Unit tests via `go test ./... -race`; integration (pg) tests behind the `integration` build tag run via `make test-integration`.

---

## File Structure

**New files:**
- `db/migrations/000043_create_user_blocks.up.sql` / `.down.sql` — table.
- `internal/block/domain/dto.go` — response + list-item DTOs, pagination.
- `internal/block/domain/errors.go` — `ErrCannotBlockSelf`, `ErrUserNotFound`.
- `internal/block/repo/repo.go` — `Repo` interface.
- `internal/block/repo/block_pg.go` — Postgres implementation.
- `internal/block/repo/block_pg_test.go` — pg integration tests (`//go:build integration`).
- `internal/block/service/service.go` — self-block guard, existence check.
- `internal/block/service/service_test.go` — service unit tests.
- `internal/block/handler/handler.go` — HTTP handlers.
- `internal/block/handler/routes.go` — `MountBlockAuthed`.
- `internal/block/handler/handler_test.go` — handler unit tests.

**Modified files:**
- `internal/ootd/repo/repo.go` — `Repo` interface: add `viewerID` to `FeedList`/`ListByUser`/`ListComments`, add `IsBlocked`.
- `internal/ootd/repo/ootd_pg.go` — implement block-filtered reads + `IsBlocked`.
- `internal/ootd/service/service.go` — thread `viewerID`; block check in `GetPost`; `ListComments` takes `viewerID`.
- `internal/ootd/service/service_test.go` — update `fakeRepo` signatures + add `blocked` field.
- `internal/ootd/handler/handler.go` — `ListComments` passes the viewer id.
- `internal/ootd/repo/ootd_pg_test.go` — block-filter regression tests.
- `cmd/api/main.go` — wire the `block` module.
- `cmd/api/main_test.go` — wire the `block` module for the API test harness.

---

## Task 1: Migration — `user_blocks` table

**Files:**
- Create: `db/migrations/000043_create_user_blocks.up.sql`
- Create: `db/migrations/000043_create_user_blocks.down.sql`

- [ ] **Step 1: Write the up migration**

`db/migrations/000043_create_user_blocks.up.sql`:

```sql
CREATE TABLE user_blocks (
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);
CREATE INDEX idx_user_blocks_blocker ON user_blocks (blocker_id);
```

- [ ] **Step 2: Write the down migration**

`db/migrations/000043_create_user_blocks.down.sql`:

```sql
DROP TABLE IF EXISTS user_blocks;
```

- [ ] **Step 3: Apply to the test DB and verify it migrates clean**

Run: `make test-db-reset`
Expected: migrate runs through version 43 with no error.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000043_create_user_blocks.up.sql db/migrations/000043_create_user_blocks.down.sql
git commit -m "feat(moderation): user_blocks table (UC40)"
```

---

## Task 2: `block` domain (DTOs + errors)

**Files:**
- Create: `internal/block/domain/dto.go`
- Create: `internal/block/domain/errors.go`

- [ ] **Step 1: Write the DTOs**

`internal/block/domain/dto.go`:

```go
// Package domain holds block DTOs and errors.
package domain

// BlockStatusResponse is returned by block/unblock endpoints.
type BlockStatusResponse struct {
	Blocked bool `json:"blocked"`
}

// BlockedUserItem is one entry in the "users I've blocked" list.
type BlockedUserItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, limit, total int) Pagination {
	tp := 0
	if limit > 0 {
		tp = (total + limit - 1) / limit
	}
	return Pagination{Page: page, Limit: limit, Total: total, TotalPages: tp}
}
```

- [ ] **Step 2: Write the errors**

`internal/block/domain/errors.go`:

```go
package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrCannotBlockSelf() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "CANNOT_BLOCK_SELF", "You cannot block yourself")
}

func ErrUserNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "User not found")
}
```

- [ ] **Step 3: Build to verify the package compiles**

Run: `go build ./internal/block/...`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add internal/block/domain
git commit -m "feat(block): domain DTOs and errors"
```

---

## Task 3: `block` repo (interface + Postgres + pg test)

**Files:**
- Create: `internal/block/repo/repo.go`
- Create: `internal/block/repo/block_pg.go`
- Test: `internal/block/repo/block_pg_test.go`

- [ ] **Step 1: Write the repo interface**

`internal/block/repo/repo.go`:

```go
// Package repo defines persistence for user blocks.
package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type Repo interface {
	UserExists(ctx context.Context, id uuid.UUID) (bool, error)
	// Block inserts (blocker, blocked); idempotent via ON CONFLICT DO NOTHING.
	Block(ctx context.Context, blocker, blocked uuid.UUID) error
	// Unblock deletes the row; idempotent (no error if absent).
	Unblock(ctx context.Context, blocker, blocked uuid.UUID) error
	ListBlocked(ctx context.Context, blocker uuid.UUID, limit, offset int) ([]domain.BlockedUserItem, int, error)
}
```

- [ ] **Step 2: Write the Postgres implementation**

`internal/block/repo/block_pg.go`:

```go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type BlockPG struct{ pool *pgxpool.Pool }

func NewBlockPG(pool *pgxpool.Pool) *BlockPG { return &BlockPG{pool: pool} }

func (r *BlockPG) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, id).Scan(&ok)
	return ok, err
}

func (r *BlockPG) Block(ctx context.Context, blocker, blocked uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		blocker, blocked)
	return err
}

func (r *BlockPG) Unblock(ctx context.Context, blocker, blocked uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`, blocker, blocked)
	return err
}

func (r *BlockPG) ListBlocked(ctx context.Context, blocker uuid.UUID, limit, offset int) ([]domain.BlockedUserItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_blocks WHERE blocker_id=$1`, blocker).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.name, u.avatar_url
		   FROM user_blocks b JOIN users u ON u.id = b.blocked_id
		  WHERE b.blocker_id=$1 AND u.deleted_at IS NULL
		  ORDER BY b.created_at DESC LIMIT $2 OFFSET $3`, blocker, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.BlockedUserItem
	for rows.Next() {
		var it domain.BlockedUserItem
		var id uuid.UUID
		if err := rows.Scan(&id, &it.Name, &it.AvatarURL); err != nil {
			return nil, 0, err
		}
		it.ID = id.String()
		out = append(out, it)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 3: Write the failing pg integration test**

`internal/block/repo/block_pg_test.go`:

```go
//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

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

func TestBlock_Idempotent_AndList(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	b := testfixtures.SeedCustomer(t, testPool)

	require.NoError(t, r.Block(ctx, a.ID, b.ID))
	require.NoError(t, r.Block(ctx, a.ID, b.ID)) // idempotent, no error

	items, total, err := r.ListBlocked(ctx, a.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, b.ID.String(), items[0].ID)

	require.NoError(t, r.Unblock(ctx, a.ID, b.ID))
	require.NoError(t, r.Unblock(ctx, a.ID, b.ID)) // idempotent
	_, total, err = r.ListBlocked(ctx, a.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)
}

func TestUserExists(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)

	ok, err := r.UserExists(ctx, a.ID)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = r.UserExists(ctx, uuid.New())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSelfBlock_RejectedByDB(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	err := r.Block(ctx, a.ID, a.ID) // violates CHECK (blocker_id <> blocked_id)
	require.Error(t, err)
}
```

- [ ] **Step 4: Run the pg tests**

Run: `make test-integration`
Expected: `block/repo` tests PASS (along with the rest of the suite).

- [ ] **Step 5: Commit**

```bash
git add internal/block/repo
git commit -m "feat(block): repo + pg implementation with idempotent block/unblock"
```

---

## Task 4: `block` service (self-block guard + existence check)

**Files:**
- Create: `internal/block/service/service.go`
- Test: `internal/block/service/service_test.go`

- [ ] **Step 1: Write the failing service unit test**

`internal/block/service/service_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type fakeRepo struct {
	userExists bool
	blocked    [][2]uuid.UUID
}

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error) { return f.userExists, nil }
func (f *fakeRepo) Block(_ context.Context, a, b uuid.UUID) error {
	f.blocked = append(f.blocked, [2]uuid.UUID{a, b})
	return nil
}
func (f *fakeRepo) Unblock(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeRepo) ListBlocked(context.Context, uuid.UUID, int, int) ([]domain.BlockedUserItem, int, error) {
	return nil, 0, nil
}

func TestBlockUser_RejectsSelf(t *testing.T) {
	svc := New(&fakeRepo{userExists: true})
	id := uuid.New()
	if _, err := svc.BlockUser(context.Background(), id, id); err == nil {
		t.Fatal("expected CANNOT_BLOCK_SELF")
	}
}

func TestBlockUser_RejectsMissingTarget(t *testing.T) {
	svc := New(&fakeRepo{userExists: false})
	if _, err := svc.BlockUser(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected USER_NOT_FOUND")
	}
}

func TestBlockUser_Success(t *testing.T) {
	f := &fakeRepo{userExists: true}
	svc := New(f)
	resp, err := svc.BlockUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !resp.Blocked || len(f.blocked) != 1 {
		t.Errorf("got resp=%+v writes=%d", resp, len(f.blocked))
	}
}

func TestUnblockUser_Success(t *testing.T) {
	svc := New(&fakeRepo{})
	resp, err := svc.UnblockUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if resp.Blocked {
		t.Errorf("got %+v, want Blocked=false", resp)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails to compile**

Run: `go test ./internal/block/service/ -run TestBlockUser -v`
Expected: FAIL — `New`/`BlockUser` undefined.

- [ ] **Step 3: Write the service**

`internal/block/service/service.go`:

```go
// Package service holds block business logic: self-block guard, existence check.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
	"github.com/wearwhere/wearwhere_be/internal/block/repo"
)

type Service struct{ repo repo.Repo }

func New(r repo.Repo) *Service { return &Service{repo: r} }

func (s *Service) BlockUser(ctx context.Context, blocker, target uuid.UUID) (*domain.BlockStatusResponse, error) {
	if blocker == target {
		return nil, domain.ErrCannotBlockSelf()
	}
	ok, err := s.repo.UserExists(ctx, target)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrUserNotFound()
	}
	if err := s.repo.Block(ctx, blocker, target); err != nil {
		return nil, err
	}
	return &domain.BlockStatusResponse{Blocked: true}, nil
}

func (s *Service) UnblockUser(ctx context.Context, blocker, target uuid.UUID) (*domain.BlockStatusResponse, error) {
	if err := s.repo.Unblock(ctx, blocker, target); err != nil {
		return nil, err
	}
	return &domain.BlockStatusResponse{Blocked: false}, nil
}

func (s *Service) ListBlocked(ctx context.Context, blocker uuid.UUID, page, limit int) ([]domain.BlockedUserItem, int, error) {
	return s.repo.ListBlocked(ctx, blocker, limit, (page-1)*limit)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/block/service/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/block/service
git commit -m "feat(block): service with self-block guard and existence check"
```

---

## Task 5: `block` handler + routes

**Files:**
- Create: `internal/block/handler/handler.go`
- Create: `internal/block/handler/routes.go`
- Test: `internal/block/handler/handler_test.go`

- [ ] **Step 1: Write the handlers**

`internal/block/handler/handler.go`:

```go
// Package handler exposes block HTTP endpoints (all customer-authed).
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/block/domain"
	"github.com/wearwhere/wearwhere_be/internal/block/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID { id, _ := authmw.UserID(c); return id }

func parsePage(c *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	return
}

func (h *Handler) BlockUser(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrUserNotFound())
		return
	}
	resp, err := h.svc.BlockUser(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) UnblockUser(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrUserNotFound())
		return
	}
	resp, err := h.svc.UnblockUser(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) ListBlocked(c *gin.Context) {
	page, limit := parsePage(c)
	items, total, err := h.svc.ListBlocked(c.Request.Context(), h.userID(c), page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	if items == nil {
		items = []domain.BlockedUserItem{}
	}
	httpx.OK(c, gin.H{"items": items, "pagination": domain.NewPagination(page, limit, total)})
}
```

- [ ] **Step 2: Write the routes**

`internal/block/handler/routes.go`:

```go
package handler

import "github.com/gin-gonic/gin"

// MountBlockAuthed registers customer-authed block routes. Caller chains RequireAuth.
func MountBlockAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/users/:id/block", h.BlockUser)
	rg.DELETE("/users/:id/block", h.UnblockUser)
	rg.GET("/me/blocks", h.ListBlocked)
}
```

- [ ] **Step 3: Write the handler unit test**

`internal/block/handler/handler_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/block/domain"
	"github.com/wearwhere/wearwhere_be/internal/block/service"
)

type fakeRepo struct{ userExists bool }

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error)  { return f.userExists, nil }
func (f *fakeRepo) Block(context.Context, uuid.UUID, uuid.UUID) error    { return nil }
func (f *fakeRepo) Unblock(context.Context, uuid.UUID, uuid.UUID) error  { return nil }
func (f *fakeRepo) ListBlocked(context.Context, uuid.UUID, int, int) ([]domain.BlockedUserItem, int, error) {
	return nil, 0, nil
}

func setup(userID uuid.UUID, userExists bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(service.New(&fakeRepo{userExists: userExists}))
	g := r.Group("/api/v1", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountBlockAuthed(g, h)
	return r
}

func TestBlockUser_Self_400(t *testing.T) {
	id := uuid.New()
	r := setup(id, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+id.String()+"/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (self-block)", w.Code)
	}
}

func TestBlockUser_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+uuid.New().String()+"/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestListBlocked_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/me/blocks", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 4: Run the handler tests**

Run: `go test ./internal/block/handler/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/block/handler
git commit -m "feat(block): HTTP handlers and routes (block/unblock/list)"
```

---

## Task 6: Wire the `block` module into the API

**Files:**
- Modify: `cmd/api/main.go` (imports near line 54-59; construction near line 122; mount near line 346)
- Modify: `cmd/api/main_test.go` (imports near line 57-62; construction + mount near line 155-161)

- [ ] **Step 1: Add block imports to `cmd/api/main.go`**

After the `followservice` import line (`cmd/api/main.go:56`), add:

```go
	blockhandler "github.com/wearwhere/wearwhere_be/internal/block/handler"
	blockrepo "github.com/wearwhere/wearwhere_be/internal/block/repo"
	blockservice "github.com/wearwhere/wearwhere_be/internal/block/service"
```

- [ ] **Step 2: Construct the block handler in `cmd/api/main.go`**

After the follow construction (`cmd/api/main.go:122-123`):

```go
	followSvc := followservice.New(followrepo.NewFollowPG(pgPool))
	followHandler := followhandler.New(followSvc)
```

add:

```go
	blockHandler := blockhandler.New(blockservice.New(blockrepo.NewBlockPG(pgPool)))
```

- [ ] **Step 3: Mount the block routes in `cmd/api/main.go`**

After the follow mount (`cmd/api/main.go:346`):

```go
	followhandler.MountFollowAuthed(reviewsAuthed, followHandler)
```

add:

```go
	blockhandler.MountBlockAuthed(reviewsAuthed, blockHandler)
```

- [ ] **Step 4: Mirror the wiring in `cmd/api/main_test.go`**

Add the same three imports after the `followservice` import (`cmd/api/main_test.go:59`). After the follow construction (`cmd/api/main_test.go:155-156`) add:

```go
	blockHandler := blockhandler.New(blockservice.New(blockrepo.NewBlockPG(pool)))
```

After the follow mount (`cmd/api/main_test.go:161`) add:

```go
	blockhandler.MountBlockAuthed(reviewsAuthed, blockHandler)
```

- [ ] **Step 5: Build the whole binary**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go cmd/api/main_test.go
git commit -m "feat(block): wire block module into the API"
```

---

## Task 7: Thread the block filter through OOTD reads

This task changes the OOTD `Repo` interface, its Postgres implementation, the service, the handler, and the service-test `fakeRepo` together (Go compiles the package as a unit). `GetPost` keeps its signature so internal ownership/like/comment checks stay unfiltered.

**Files:**
- Modify: `internal/ootd/repo/repo.go`
- Modify: `internal/ootd/repo/ootd_pg.go`
- Modify: `internal/ootd/service/service.go`
- Modify: `internal/ootd/handler/handler.go`
- Modify: `internal/ootd/service/service_test.go`

- [ ] **Step 1: Update the OOTD `Repo` interface**

In `internal/ootd/repo/repo.go`, change these method signatures and add `IsBlocked`:

```go
	FeedList(ctx context.Context, viewerID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error)
	ListByUser(ctx context.Context, viewerID, userID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error)
```

```go
	ListComments(ctx context.Context, viewerID, postID uuid.UUID, limit, offset int) ([]*domain.CommentView, int, error)
```

Add to the interface (e.g. after `CommentOwner`):

```go
	// IsBlocked reports whether blocker has blocked blocked.
	IsBlocked(ctx context.Context, blocker, blocked uuid.UUID) (bool, error)
```

Leave `GetPost(ctx, id)`, `FollowedFeed(ctx, viewerID, limit, offset)` signatures unchanged.

- [ ] **Step 2: Rewrite `feedQuery` and the feed methods in `ootd_pg.go`**

Add `"fmt"` to the import block in `internal/ootd/repo/ootd_pg.go`. Replace the existing `feedQuery`, `FeedList`, and `ListByUser` (lines 67-112) with:

```go
// feedQuery runs count + list for a feed view. viewerID filters out posts by
// users the viewer has blocked (uuid.Nil → empty subquery → no filtering).
// userID, when non-nil, restricts to a single author (the by-user feed).
func (r *OOTDPg) feedQuery(ctx context.Context, viewerID uuid.UUID, userID *uuid.UUID, limit, offset int) ([]*domain.PostView, int, error) {
	where := `p.deleted_at IS NULL AND p.status='published'
	          AND p.user_id NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id=$1)`
	args := []any{viewerID}
	if userID != nil {
		where += ` AND p.user_id=$2`
		args = append(args, *userID)
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ootd_posts p WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	n := len(args)
	q := fmt.Sprintf(postSelect+` WHERE `+where+` ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d`, n+1, n+2)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.PostView
	for rows.Next() {
		v, err := scanPostView(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *OOTDPg) FeedList(ctx context.Context, viewerID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error) {
	return r.feedQuery(ctx, viewerID, nil, limit, offset)
}

func (r *OOTDPg) ListByUser(ctx context.Context, viewerID, userID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error) {
	return r.feedQuery(ctx, viewerID, &userID, limit, offset)
}
```

- [ ] **Step 3: Add the block subquery to `FollowedFeed`**

In `internal/ootd/repo/ootd_pg.go` `FollowedFeed`, add the block clause to BOTH the count query and the list query (the follower id is already `$1`). The count query `WHERE` becomes:

```go
		  WHERE uf.follower_id=$1 AND p.deleted_at IS NULL AND p.status='published'
		    AND p.user_id NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id=$1)
```

and the list query `WHERE` becomes:

```go
		  WHERE uf.follower_id=$1 AND p.deleted_at IS NULL AND p.status='published'
		    AND p.user_id NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id=$1)
		  ORDER BY p.created_at DESC LIMIT $2 OFFSET $3
```

- [ ] **Step 4: Add `viewerID` filtering to `ListComments` + add `IsBlocked`**

Replace `ListComments` (lines 241-267) in `internal/ootd/repo/ootd_pg.go` with:

```go
func (r *OOTDPg) ListComments(ctx context.Context, viewerID, postID uuid.UUID, limit, offset int) ([]*domain.CommentView, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ootd_comments
		  WHERE post_id=$1 AND deleted_at IS NULL AND status='published'
		    AND user_id NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id=$2)`, postID, viewerID).
		Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.post_id, c.user_id, c.body, c.status, c.created_at, u.name
		   FROM ootd_comments c JOIN users u ON u.id = c.user_id
		  WHERE c.post_id=$1 AND c.deleted_at IS NULL AND c.status='published'
		    AND c.user_id NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id=$2)
		  ORDER BY c.created_at ASC
		  LIMIT $3 OFFSET $4`, postID, viewerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.CommentView
	for rows.Next() {
		var v domain.CommentView
		if err := rows.Scan(&v.ID, &v.PostID, &v.UserID, &v.Body, &v.Status, &v.CreatedAt, &v.AuthorName); err != nil {
			return nil, 0, err
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

func (r *OOTDPg) IsBlocked(ctx context.Context, blocker, blocked uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2)`, blocker, blocked).Scan(&ok)
	return ok, err
}
```

- [ ] **Step 5: Update the OOTD service**

In `internal/ootd/service/service.go`:

`Feed` — pass `viewerID`:

```go
func (s *Service) Feed(ctx context.Context, viewerID uuid.UUID, page, limit int) ([]*domain.PostView, int, error) {
	views, total, err := s.repo.FeedList(ctx, viewerID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, views, viewerID); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}
```

`ByUser` — pass `viewerID`:

```go
func (s *Service) ByUser(ctx context.Context, viewerID, userID uuid.UUID, page, limit int) ([]*domain.PostView, int, error) {
	views, total, err := s.repo.ListByUser(ctx, viewerID, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, views, viewerID); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}
```

`GetPost` — block check for an authenticated viewer:

```go
func (s *Service) GetPost(ctx context.Context, viewerID, postID uuid.UUID) (*domain.PostView, error) {
	v, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return nil, domain.ErrPostNotFound()
	}
	if viewerID != uuid.Nil {
		blocked, err := s.repo.IsBlocked(ctx, viewerID, v.UserID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, domain.ErrPostNotFound()
		}
	}
	if err := s.enrich(ctx, []*domain.PostView{v}, viewerID); err != nil {
		return nil, err
	}
	return v, nil
}
```

`ListComments` — take `viewerID`:

```go
func (s *Service) ListComments(ctx context.Context, viewerID, postID uuid.UUID, page, limit int) ([]*domain.CommentView, int, error) {
	return s.repo.ListComments(ctx, viewerID, postID, limit, (page-1)*limit)
}
```

- [ ] **Step 6: Update the OOTD handler `ListComments`**

In `internal/ootd/handler/handler.go` `ListComments`, change the service call to pass the viewer id:

```go
	list, total, err := h.svc.ListComments(c.Request.Context(), h.userID(c), id, page, limit)
```

- [ ] **Step 7: Update the service-test `fakeRepo`**

In `internal/ootd/service/service_test.go`, update the `fakeRepo` to match the new interface. Add a `blocked` field to the struct:

```go
type fakeRepo struct {
	created   *domain.Post
	createErr error
	owner     uuid.UUID
	post      *domain.PostView
	blocked   bool
}
```

Replace the `FeedList`, `ListByUser`, and `ListComments` methods, and add `IsBlocked`:

```go
func (f *fakeRepo) FeedList(context.Context, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListByUser(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListComments(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*domain.CommentView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) IsBlocked(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.blocked, nil
}
```

- [ ] **Step 8: Add a service unit test for the Detail block check**

Append to `internal/ootd/service/service_test.go`:

```go
func TestGetPost_HiddenWhenViewerBlockedAuthor(t *testing.T) {
	author := uuid.New()
	f := &fakeRepo{
		post:    &domain.PostView{Post: domain.Post{ID: uuid.New(), UserID: author}},
		blocked: true,
	}
	svc := newSvc(f, &memStorage{})
	if _, err := svc.GetPost(context.Background(), uuid.New(), f.post.ID); err == nil {
		t.Fatal("expected POST_NOT_FOUND when viewer has blocked the author")
	}
}

func TestGetPost_VisibleToGuest(t *testing.T) {
	f := &fakeRepo{
		post:    &domain.PostView{Post: domain.Post{ID: uuid.New(), UserID: uuid.New()}},
		blocked: true, // ignored: guest viewer (uuid.Nil) skips the block check
	}
	svc := newSvc(f, &memStorage{})
	if _, err := svc.GetPost(context.Background(), uuid.Nil, f.post.ID); err != nil {
		t.Fatalf("guest should see the post: %v", err)
	}
}
```

- [ ] **Step 9: Run OOTD unit tests + full build**

Run: `go build ./... && go test ./internal/ootd/... -race`
Expected: build succeeds; OOTD service/handler unit tests PASS (including the two new ones).

- [ ] **Step 10: Commit**

```bash
git add internal/ootd cmd/api
git commit -m "feat(moderation): filter blocked users' OOTD content from viewer reads"
```

---

## Task 8: OOTD pg regression tests for block filtering

**Files:**
- Modify: `internal/ootd/repo/ootd_pg_test.go`

- [ ] **Step 1: Write the failing pg integration tests**

Append to `internal/ootd/repo/ootd_pg_test.go`. These rely on the existing `makePost` helper (creates a post owned by a fresh customer; returns post id + owner id) and `testfixtures.SeedCustomer`:

```go
func TestBlock_HidesPostFromFeedAndByUserAndDetail(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)

	postID, author := makePost(t, r, nil)
	viewer := testfixtures.SeedCustomer(t, testPool)

	// Before blocking: visible in feed, by-user, and detail-eligible.
	posts, total, err := r.FeedList(ctx, viewer.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, posts, 1)

	// Block the author.
	_, err = testPool.Exec(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1,$2)`, viewer.ID, author)
	require.NoError(t, err)

	// Feed now empty for the blocker.
	posts, total, err = r.FeedList(ctx, viewer.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)
	require.Len(t, posts, 0)

	// By-user listing of the blocked author is empty for the blocker.
	_, total, err = r.ListByUser(ctx, viewer.ID, author, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)

	// IsBlocked reports the relationship (drives the Detail 404 in the service).
	blocked, err := r.IsBlocked(ctx, viewer.ID, author)
	require.NoError(t, err)
	require.True(t, blocked)

	// A different viewer (and guest, uuid.Nil) still sees the post.
	other := testfixtures.SeedCustomer(t, testPool)
	_, total, err = r.FeedList(ctx, other.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.NotEqual(t, uuid.Nil, postID) // postID is used by the assertions above
}

func TestBlock_HidesCommentsFromBlocker(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)

	postID, _ := makePost(t, r, nil)
	commenter := testfixtures.SeedCustomer(t, testPool)
	viewer := testfixtures.SeedCustomer(t, testPool)

	require.NoError(t, r.AddComment(ctx, &domain.Comment{PostID: postID, UserID: commenter.ID, Body: "nice"}))

	// Visible before block.
	_, total, err := r.ListComments(ctx, viewer.ID, postID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)

	// Viewer blocks the commenter.
	_, err = testPool.Exec(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1,$2)`, viewer.ID, commenter.ID)
	require.NoError(t, err)

	// Comment hidden from the blocker.
	_, total, err = r.ListComments(ctx, viewer.ID, postID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)

	// Still visible to a guest.
	_, total, err = r.ListComments(ctx, uuid.Nil, postID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
}
```

- [ ] **Step 2: Run the full integration suite**

Run: `make test-integration`
Expected: all tests PASS, including the two new block-filter tests.

- [ ] **Step 3: Commit**

```bash
git add internal/ootd/repo/ootd_pg_test.go
git commit -m "test(moderation): pg regression for block filtering on OOTD reads"
```

---

## Task 9: Final verification

- [ ] **Step 1: Build + unit tests**

Run: `go build ./... && go test ./... -race`
Expected: build succeeds; all non-integration tests PASS.

- [ ] **Step 2: Full integration suite**

Run: `make test-integration`
Expected: all tests PASS.

- [ ] **Step 3: Confirm no uncommitted changes**

Run: `git status`
Expected: clean working tree (everything committed across Tasks 1-8).
```

---

## Self-Review

**Spec coverage:**
- UC40 block/unblock → Tasks 2-6. ✓
- "Hide their content from me" across Feed/Following/ByUser/Comments/Detail → Task 7 (all five read paths) + Task 8 (regression). ✓
- `user_blocks` schema → Task 1, matches spec exactly. ✓
- Self-block rejected, missing-target rejected, idempotent block/unblock → Tasks 3-4. ✓
- Detail returns 404 for blocked author, visible to others/guests → Task 7 Step 8 + Task 8. ✓
- Admin/report cut → no tasks, as intended. ✓

**Type consistency:** `BlockStatusResponse{Blocked}`, `BlockedUserItem{ID,Name,AvatarURL}`, repo methods `UserExists/Block/Unblock/ListBlocked`, OOTD `FeedList(viewerID,...)`, `ListByUser(viewerID,userID,...)`, `ListComments(viewerID,postID,...)`, `IsBlocked(blocker,blocked)` — used identically across interface, pg impl, service, fakes, and tests.

**Note on `DELETE` response:** mirrors the existing `follow` convention — `200 OK` with `{"blocked": false}` rather than `204` (the spec mentioned 204; this refinement keeps the codebase consistent with `Unfollow`). Flag for the user if strict 204 is preferred.

**Placeholder scan:** none — every step ships complete code/SQL/commands.
