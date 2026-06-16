# UC31 Set Style Preferences — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a per-user style profile (preferred style tags + budget range) that the recommendation feed (UC29) and the AI stylist (UC28 B1 seam) consume for personalization.

**Architecture:** A new `internal/styleprofile` module following the existing layered pattern (domain → repo → service → handler), mounted on the existing `/api/v1/me` customer group. One `style_profiles` row per user plus an M-N `style_profile_tags` table referencing the existing `style_tags` table that products are already tagged with. Favorite brands are NOT stored here — brand-follow (UC35) is the signal.

**Tech Stack:** Go 1.23, gin, pgx/v5 (PostgreSQL), `pkg/httpx` (Format A errors), testify. Pure-logic tests run untagged with fakes; DB tests use `//go:build integration` + `TEST_DATABASE_URL` + `internal/testfixtures`.

**Spec:** `docs/superpowers/specs/2026-06-16-ai-personalization-design.md` (§3).

**Conventions (verified against the codebase):**
- Errors: services return `*httpx.AppError` (or a typed error the handler maps); handlers call `httpx.ErrorFromApp(c, err)` / `httpx.ErrorWithDetails(...)`. Success: `httpx.OK`.
- Auth: handler reads caller via `authmw.UserID(c)` (`authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"`). Mounted on the `/me` group that already applies `RequireAuth` + `RequireRole(customer)`.
- Repos: `DBTX` interface; `New<Thing>PG(db)` constructor; sentinel `repo.ErrNotFound`.
- Time in responses: `t.UTC().Format("2006-01-02T15:04:05Z")` (matches wishlist).
- Module path prefix: `github.com/wearwhere/wearwhere_be`.

**File structure:**
```
db/migrations/
  000044_create_style_profiles.{up,down}.sql        NEW
  000045_create_style_profile_tags.{up,down}.sql    NEW
internal/styleprofile/
  domain/
    profile.go      StyleProfileView, StyleTagRef entities
    dto.go          UpdateStyleProfileRequest, StyleProfileResponse, UpsertParams
    errors.go       ErrInvalidBudget, UnknownStyleTagsError
  repo/
    repo.go         DBTX, ErrNotFound, StyleProfileRepo interface
    style_profile_pg.go        Load / Upsert / UnknownTagIDs
    style_profile_pg_test.go   (integration)
  service/
    service.go      Get / Save (validation) + LoadProfile getter
    service_test.go (unit, fake repo)
  handler/
    handler.go      GET / PUT
    routes.go       Mount
    handler_test.go (unit, fake service)
cmd/api/main.go     MODIFY  construct + mount the module
```

> **Migration numbers:** `000043` is the latest existing migration. If `000044`/`000045` are already taken when you start (e.g. UC28 landed first), use the next free sequential pair and keep both files consistent.

---

## Task 1: Database migrations

**Files:**
- Create: `db/migrations/000044_create_style_profiles.up.sql`
- Create: `db/migrations/000044_create_style_profiles.down.sql`
- Create: `db/migrations/000045_create_style_profile_tags.up.sql`
- Create: `db/migrations/000045_create_style_profile_tags.down.sql`

- [ ] **Step 1: Write `000044_create_style_profiles.up.sql`**

```sql
CREATE TABLE style_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    budget_min   INTEGER,
    budget_max   INTEGER,
    onboarded_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (budget_min IS NULL OR budget_min >= 0),
    CHECK (budget_max IS NULL OR budget_min IS NULL OR budget_max >= budget_min)
);
```

- [ ] **Step 2: Write `000044_create_style_profiles.down.sql`**

```sql
DROP TABLE IF EXISTS style_profiles;
```

- [ ] **Step 3: Write `000045_create_style_profile_tags.up.sql`**

```sql
CREATE TABLE style_profile_tags (
    user_id      UUID NOT NULL REFERENCES style_profiles(user_id) ON DELETE CASCADE,
    style_tag_id UUID NOT NULL REFERENCES style_tags(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, style_tag_id)
);
CREATE INDEX idx_style_profile_tags_user ON style_profile_tags (user_id);
```

- [ ] **Step 4: Write `000045_create_style_profile_tags.down.sql`**

```sql
DROP TABLE IF EXISTS style_profile_tags;
```

- [ ] **Step 5: Apply migrations against the dev DB and verify**

Run (matches the dev docker-compose migrate flow):
```bash
docker compose run --rm migrate
```
Expected: migrations `000044` and `000045` applied with no error. Verify:
```bash
docker compose exec postgres psql -U wearwhere -d wearwhere -c "\d style_profiles" -c "\d style_profile_tags"
```
Expected: both tables exist with the columns above.

- [ ] **Step 6: Commit**

```bash
git add db/migrations/000044_* db/migrations/000045_*
git commit -m "feat(styleprofile): migrations for style_profiles + style_profile_tags"
```

---

## Task 2: Domain entities, DTOs, errors

**Files:**
- Create: `internal/styleprofile/domain/profile.go`
- Create: `internal/styleprofile/domain/dto.go`
- Create: `internal/styleprofile/domain/errors.go`

- [ ] **Step 1: Write `profile.go`**

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// StyleTagRef is the public shape of a style tag (id/slug/name), matching the
// product catalog's StyleTagRef fields.
type StyleTagRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// StyleProfileView is the assembled profile returned to callers and consumed
// in-process by the recommendation service. A user with no saved profile is
// represented by a zero-value view (empty StyleTags, nil budgets, nil OnboardedAt).
type StyleProfileView struct {
	UserID      uuid.UUID
	StyleTags   []StyleTagRef
	BudgetMin   *int
	BudgetMax   *int
	OnboardedAt *time.Time
}
```

- [ ] **Step 2: Write `dto.go`**

```go
package domain

import "github.com/google/uuid"

// UpdateStyleProfileRequest is the PUT body. Budget cross-field validation
// (max >= min) is done in the service, not via binding tags, so nil budgets
// are handled cleanly.
type UpdateStyleProfileRequest struct {
	StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
	BudgetMin   *int     `json:"budget_min"    binding:"omitempty,gte=0"`
	BudgetMax   *int     `json:"budget_max"    binding:"omitempty,gte=0"`
}

// StyleProfileResponse is the GET/PUT response body.
type StyleProfileResponse struct {
	StyleTags   []StyleTagRef `json:"style_tags"`
	BudgetMin   *int          `json:"budget_min,omitempty"`
	BudgetMax   *int          `json:"budget_max,omitempty"`
	OnboardedAt *string       `json:"onboarded_at,omitempty"`
}

// UpsertParams is the repo input for a profile write.
type UpsertParams struct {
	UserID      uuid.UUID
	StyleTagIDs []uuid.UUID
	BudgetMin   *int
	BudgetMax   *int
}
```

- [ ] **Step 3: Write `errors.go`**

```go
package domain

import (
	"net/http"
	"strings"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// ErrInvalidBudget is returned when budget_max < budget_min.
var ErrInvalidBudget = httpx.NewAppError(
	http.StatusBadRequest, "VALIDATION_FAILED", "budget_max must be >= budget_min",
)

// UnknownStyleTagsError carries the style tag IDs that do not exist so the
// handler can surface them in the Format-A error details.
type UnknownStyleTagsError struct{ IDs []string }

func (e *UnknownStyleTagsError) Error() string {
	return "unknown style tag ids: " + strings.Join(e.IDs, ",")
}
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./internal/styleprofile/...`
Expected: no output (success). It compiles even though repo/service/handler don't exist yet because this package has no internal deps beyond `httpx`.

- [ ] **Step 5: Commit**

```bash
git add internal/styleprofile/domain
git commit -m "feat(styleprofile): domain entities, DTOs, errors"
```

---

## Task 3: Repository (interface + Postgres + integration tests)

**Files:**
- Create: `internal/styleprofile/repo/repo.go`
- Create: `internal/styleprofile/repo/style_profile_pg.go`
- Test: `internal/styleprofile/repo/style_profile_pg_test.go`

- [ ] **Step 1: Write the interface `repo.go`**

```go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

// ErrNotFound means the user has no saved style profile row.
var ErrNotFound = errors.New("styleprofile: not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type StyleProfileRepo interface {
	// Load returns the assembled view or ErrNotFound when no profile row exists.
	Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error)
	// Upsert atomically writes the profile row and replaces its tag set, then
	// returns the freshly-loaded view. onboarded_at is set on first insert and
	// preserved thereafter.
	Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error)
	// UnknownTagIDs returns the subset of ids that are NOT present in style_tags.
	UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}
```

- [ ] **Step 2: Write the Postgres implementation `style_profile_pg.go`**

```go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

type StyleProfilePG struct{ db DBTX }

func NewStyleProfilePG(db DBTX) *StyleProfilePG { return &StyleProfilePG{db: db} }

func (r *StyleProfilePG) UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT x FROM unnest($1::uuid[]) AS x
		 WHERE NOT EXISTS (SELECT 1 FROM style_tags st WHERE st.id = x)`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Upsert runs the profile write + tag replacement as a single multi-statement
// query. Postgres executes every data-modifying CTE exactly once and to
// completion, so the delete+insert of tags is atomic even on a connection pool.
func (r *StyleProfilePG) Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	_, err := r.db.Exec(ctx,
		`WITH up AS (
		    INSERT INTO style_profiles (user_id, budget_min, budget_max, onboarded_at)
		    VALUES ($1, $2, $3, NOW())
		    ON CONFLICT (user_id) DO UPDATE
		      SET budget_min = EXCLUDED.budget_min,
		          budget_max = EXCLUDED.budget_max,
		          updated_at = NOW()
		    RETURNING user_id
		 ),
		 del AS (
		    DELETE FROM style_profile_tags WHERE user_id = $1
		 )
		 INSERT INTO style_profile_tags (user_id, style_tag_id)
		 SELECT $1, t FROM unnest($4::uuid[]) AS t
		 ON CONFLICT DO NOTHING`,
		p.UserID, p.BudgetMin, p.BudgetMax, p.StyleTagIDs)
	if err != nil {
		return nil, err
	}
	return r.Load(ctx, p.UserID)
}

func (r *StyleProfilePG) Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v := &domain.StyleProfileView{UserID: userID}
	err := r.db.QueryRow(ctx,
		`SELECT budget_min, budget_max, onboarded_at
		   FROM style_profiles WHERE user_id = $1`, userID).
		Scan(&v.BudgetMin, &v.BudgetMax, &v.OnboardedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT st.id, st.slug, st.name
		   FROM style_profile_tags spt
		   JOIN style_tags st ON st.id = spt.style_tag_id
		  WHERE spt.user_id = $1
		  ORDER BY st.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref domain.StyleTagRef
		var id uuid.UUID
		if err := rows.Scan(&id, &ref.Slug, &ref.Name); err != nil {
			return nil, err
		}
		ref.ID = id.String()
		v.StyleTags = append(v.StyleTags, ref)
	}
	return v, rows.Err()
}
```

- [ ] **Step 3: Write the failing integration test `style_profile_pg_test.go`**

```go
//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func intp(v int) *int { return &v }

func TestStyleProfilePG_LoadNotFound(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewStyleProfilePG(tx)

	_, err := r.Load(context.Background(), user.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestStyleProfilePG_UpsertSetsOnboardedAndPreservesIt(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	tag1 := testfixtures.SeedStyleTag(t, tx)
	tag2 := testfixtures.SeedStyleTag(t, tx)
	r := repo.NewStyleProfilePG(tx)

	v1, err := r.Upsert(ctx, domain.UpsertParams{
		UserID: user.ID, StyleTagIDs: []uuid.UUID{tag1.ID}, BudgetMin: intp(100000), BudgetMax: intp(500000),
	})
	require.NoError(t, err)
	require.NotNil(t, v1.OnboardedAt)
	require.Len(t, v1.StyleTags, 1)
	require.Equal(t, 100000, *v1.BudgetMin)
	firstOnboarded := *v1.OnboardedAt

	// Second upsert replaces the tag set and keeps onboarded_at.
	v2, err := r.Upsert(ctx, domain.UpsertParams{
		UserID: user.ID, StyleTagIDs: []uuid.UUID{tag2.ID}, BudgetMin: nil, BudgetMax: nil,
	})
	require.NoError(t, err)
	require.Len(t, v2.StyleTags, 1)
	require.Equal(t, tag2.ID.String(), v2.StyleTags[0].ID)
	require.Nil(t, v2.BudgetMin)
	require.WithinDuration(t, firstOnboarded, *v2.OnboardedAt, 0)
}

func TestStyleProfilePG_UnknownTagIDs(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	tag := testfixtures.SeedStyleTag(t, tx)
	missing := uuid.New()
	r := repo.NewStyleProfilePG(tx)

	unknown, err := r.UnknownTagIDs(ctx, []uuid.UUID{tag.ID, missing})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{missing}, unknown)
}
```

- [ ] **Step 4: Run the integration tests to verify they pass**

Run:
```bash
go test -tags=integration ./internal/styleprofile/repo/... -v
```
(Requires `TEST_DATABASE_URL` pointing at a migrated test DB — same as the wishlist repo tests.)
Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/styleprofile/repo
git commit -m "feat(styleprofile): repo with atomic upsert + tag validation"
```

---

## Task 4: Service (validation + getters)

**Files:**
- Create: `internal/styleprofile/service/service.go`
- Test: `internal/styleprofile/service/service_test.go`

- [ ] **Step 1: Write the failing unit test `service_test.go`**

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
)

type fakeRepo struct {
	loadErr   error
	view      *domain.StyleProfileView
	unknown   []uuid.UUID
	upserted  *domain.UpsertParams
}

func (f *fakeRepo) Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.view, nil
}
func (f *fakeRepo) Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	f.upserted = &p
	return &domain.StyleProfileView{UserID: p.UserID, BudgetMin: p.BudgetMin, BudgetMax: p.BudgetMax}, nil
}
func (f *fakeRepo) UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	return f.unknown, nil
}

func intp(v int) *int { return &v }

func TestGet_EmptyWhenNoProfile(t *testing.T) {
	svc := service.New(&fakeRepo{loadErr: repo.ErrNotFound})
	uid := uuid.New()
	v, err := svc.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, uid, v.UserID)
	require.Empty(t, v.StyleTags)
	require.Nil(t, v.OnboardedAt)
}

func TestSave_RejectsBadBudget(t *testing.T) {
	svc := service.New(&fakeRepo{})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		BudgetMin: intp(500000), BudgetMax: intp(100000),
	})
	require.ErrorIs(t, err, domain.ErrInvalidBudget)
}

func TestSave_RejectsUnknownTags(t *testing.T) {
	bad := uuid.New()
	svc := service.New(&fakeRepo{unknown: []uuid.UUID{bad}})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{bad.String()},
	})
	var ute *domain.UnknownStyleTagsError
	require.ErrorAs(t, err, &ute)
	require.Equal(t, []string{bad.String()}, ute.IDs)
}

func TestSave_InvalidUUIDInTagsIsRejected(t *testing.T) {
	svc := service.New(&fakeRepo{})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{"not-a-uuid"},
	})
	require.Error(t, err)
}

func TestSave_PassesParsedParamsToRepo(t *testing.T) {
	tag := uuid.New()
	f := &fakeRepo{}
	svc := service.New(f)
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{tag.String()}, BudgetMin: intp(100000),
	})
	require.NoError(t, err)
	require.NotNil(t, f.upserted)
	require.Equal(t, []uuid.UUID{tag}, f.upserted.StyleTagIDs)
	require.Equal(t, 100000, *f.upserted.BudgetMin)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/styleprofile/service/... -v`
Expected: FAIL — `service.New` / `Get` / `Save` undefined.

- [ ] **Step 3: Write the implementation `service.go`**

```go
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
)

type Service struct{ repo repo.StyleProfileRepo }

func New(r repo.StyleProfileRepo) *Service { return &Service{repo: r} }

// Get returns the saved profile, or an empty (zero-value) view when the user
// has never set one. GET never 404s on a missing profile.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v, err := s.repo.Load(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return &domain.StyleProfileView{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// LoadProfile is the in-process getter for other services (recommendation,
// stylist). It returns nil when the user has no profile.
func (s *Service) LoadProfile(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v, err := s.repo.Load(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}
	return v, err
}

// Save validates and upserts the profile. Idempotent: fully overwrites the
// tag set and budget. Forward note: when UC29 lands, invalidate that user's
// recommendation cache here after a successful upsert.
func (s *Service) Save(ctx context.Context, userID uuid.UUID, req domain.UpdateStyleProfileRequest) (*domain.StyleProfileView, error) {
	if req.BudgetMin != nil && req.BudgetMax != nil && *req.BudgetMax < *req.BudgetMin {
		return nil, domain.ErrInvalidBudget
	}

	ids := make([]uuid.UUID, 0, len(req.StyleTagIDs))
	for _, s := range req.StyleTagIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, &domain.UnknownStyleTagsError{IDs: []string{s}}
		}
		ids = append(ids, id)
	}

	if len(ids) > 0 {
		unknown, err := s.repo.UnknownTagIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			out := make([]string, len(unknown))
			for i, u := range unknown {
				out[i] = u.String()
			}
			return nil, &domain.UnknownStyleTagsError{IDs: out}
		}
	}

	return s.repo.Upsert(ctx, domain.UpsertParams{
		UserID:      userID,
		StyleTagIDs: ids,
		BudgetMin:   req.BudgetMin,
		BudgetMax:   req.BudgetMax,
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/styleprofile/service/... -v`
Expected: PASS for all five tests.

- [ ] **Step 5: Commit**

```bash
git add internal/styleprofile/service
git commit -m "feat(styleprofile): service with budget + tag validation and LoadProfile getter"
```

---

## Task 5: Handler + routes

**Files:**
- Create: `internal/styleprofile/handler/handler.go`
- Create: `internal/styleprofile/handler/routes.go`
- Test: `internal/styleprofile/handler/handler_test.go`

- [ ] **Step 1: Write the handler `handler.go`**

```go
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func viewToResponse(v *domain.StyleProfileView) domain.StyleProfileResponse {
	resp := domain.StyleProfileResponse{
		StyleTags: v.StyleTags,
		BudgetMin: v.BudgetMin,
		BudgetMax: v.BudgetMax,
	}
	if resp.StyleTags == nil {
		resp.StyleTags = []domain.StyleTagRef{}
	}
	if v.OnboardedAt != nil {
		s := v.OnboardedAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.OnboardedAt = &s
	}
	return resp
}

func (h *Handler) Get(c *gin.Context) {
	v, err := h.svc.Get(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, viewToResponse(v))
}

func (h *Handler) Put(c *gin.Context) {
	var req domain.UpdateStyleProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	v, err := h.svc.Save(c.Request.Context(), h.userID(c), req)
	if err != nil {
		var ute *domain.UnknownStyleTagsError
		if errors.As(err, &ute) {
			httpx.ErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_FAILED",
				"One or more style tags do not exist",
				map[string]any{"unknown_style_tag_ids": ute.IDs})
			return
		}
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, viewToResponse(v))
}
```

- [ ] **Step 2: Write the routes `routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

// Mount registers style-profile routes under a group that already applies
// RequireAuth + RequireRole(customer) (the /me customer group).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/style-profile", h.Get)
	rg.PUT("/style-profile", h.Put)
}
```

- [ ] **Step 3: Write the failing handler test `handler_test.go`**

This test wires the handler to a real service backed by a fake repo, injects a user id into the gin context the way `authmw.UserID` reads it, and exercises both endpoints over httptest.

> **Before writing:** open `internal/auth/middleware` and confirm the context key `authmw.UserID` reads (e.g. `c.Get("user_id")` or a typed key). Set the SAME key in the test below where marked, so `h.userID(c)` resolves. Mirror exactly what `internal/wishlist/handler/handler_test.go` does to set the caller.

```go
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/handler"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

// fakeRepo: never-found Load, echoing Upsert, configurable unknown tags.
type fakeRepo struct{ unknown []uuid.UUID }

func (f *fakeRepo) Load(_ ctxT, _ uuid.UUID) (*domain.StyleProfileView, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeRepo) Upsert(_ ctxT, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	return &domain.StyleProfileView{UserID: p.UserID, BudgetMin: p.BudgetMin}, nil
}
func (f *fakeRepo) UnknownTagIDs(_ ctxT, _ []uuid.UUID) ([]uuid.UUID, error) {
	return f.unknown, nil
}

func setup(f *fakeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := handler.New(service.New(f))
	r := gin.New()
	// Inject a caller user_id the same way the real auth middleware does.
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uuid.New()) // TODO: match the exact key/type authmw.UserID reads
		c.Next()
	})
	handler.Mount(r.Group("/me"), h)
	return r
}

func TestGet_EmptyProfile200(t *testing.T) {
	r := setup(&fakeRepo{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/style-profile", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var body domain.StyleProfileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, []domain.StyleTagRef{}, body.StyleTags)
}

func TestPut_UnknownTag400WithDetails(t *testing.T) {
	bad := uuid.New()
	r := setup(&fakeRepo{unknown: []uuid.UUID{bad}})
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"style_tag_ids": []string{bad.String()}})
	req, _ := http.NewRequest(http.MethodPut, "/me/style-profile", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "VALIDATION_FAILED")
	require.Contains(t, w.Body.String(), bad.String())
}

func TestPut_BadBudget400(t *testing.T) {
	r := setup(&fakeRepo{})
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"budget_min": 500000, "budget_max": 100000})
	req, _ := http.NewRequest(http.MethodPut, "/me/style-profile", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
```

> **Note:** `ctxT` is shorthand — replace with `context.Context` and add the `context` import. It is written this way only to flag that the fake's signatures must match `repo.StyleProfileRepo` exactly.

- [ ] **Step 4: Run the handler tests; fix the context key, then verify pass**

Run: `go test ./internal/styleprofile/handler/... -v`
Expected: PASS. If `Get` returns a non-empty user id mismatch or panics, the context key in `setup` does not match what `authmw.UserID` reads — fix it to match the real middleware and re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/styleprofile/handler
git commit -m "feat(styleprofile): GET/PUT handler + routes"
```

---

## Task 6: Wire the module into the API

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add the import block**

In the grouped imports of `cmd/api/main.go`, alongside the other module imports (e.g. near the `wishlist*` imports), add:

```go
	styleprofilehandler "github.com/wearwhere/wearwhere_be/internal/styleprofile/handler"
	styleprofilerepo "github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	styleprofileservice "github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
```

- [ ] **Step 2: Construct the module**

Near where `wishlistSvc` / `wishlistHandler` are constructed (around `main.go:193` and `main.go:293`), add:

```go
	styleProfileSvc := styleprofileservice.New(styleprofilerepo.NewStyleProfilePG(pgPool))
	styleProfileHandler := styleprofilehandler.New(styleProfileSvc)
```

- [ ] **Step 3: Mount on the customer group**

In the `customerGroup` mount block (around `main.go:338-345`, after `wishlisthandler.Mount(customerGroup, wishlistHandler)`), add:

```go
	styleprofilehandler.Mount(customerGroup, styleProfileHandler)
```

- [ ] **Step 4: Build the API**

Run: `go build ./cmd/api/...`
Expected: no output (success).

- [ ] **Step 5: Run the full unit test suite**

Run: `go test ./internal/styleprofile/...`
Expected: PASS (handler + service untagged tests). The repo integration tests need `-tags=integration` + DB.

- [ ] **Step 6: Manual smoke test (optional but recommended)**

Start the stack (`docker compose up -d --build`), obtain a customer access token via login, then:
```bash
# Set a profile
curl -sS -X PUT localhost:8080/api/v1/me/style-profile \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"style_tag_ids":["<real-style-tag-uuid>"],"budget_min":100000,"budget_max":800000}'
# Read it back
curl -sS localhost:8080/api/v1/me/style-profile -H "Authorization: Bearer $TOKEN"
```
Expected: PUT returns the saved profile with `onboarded_at`; GET returns the same tags/budget.

- [ ] **Step 7: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(styleprofile): wire style-profile module into the API"
```

---

## Self-Review

**Spec coverage (§3 of the design):**
- Data model `style_profiles` + `style_profile_tags` referencing `style_tags` → Task 1. ✓
- Favorite brands NOT stored here → no table/column for it (brand-follow stays the signal). ✓
- `GET /me/style-profile` returns profile or empty object → Task 5 handler + Task 4 `Get`. ✓
- `PUT /me/style-profile` upsert, sets `onboarded_at` first time → Task 3 Upsert CTE + Task 4. ✓
- Validation: unknown tag id → `VALIDATION_FAILED` with offending ids in details → Task 4 + Task 5. ✓
- Validation: `budget_max >= budget_min` → Task 4 `ErrInvalidBudget`. ✓
- Max 10 tags → binding tag `max=10` in DTO (Task 2). ✓
- Editable anytime (idempotent overwrite) → Upsert replaces tag set + budget. ✓
- Internal `LoadProfile(userID)` getter for other services → Task 4. ✓
- Cache invalidation on PUT → forward note in Task 4 `Save` (UC29 cache does not exist yet; hooked when UC29 lands). ✓ (documented dependency, not a gap)

**Placeholder scan:** The handler test uses `c.Set("user_id", ...)` marked TODO to match the real auth middleware key — this is an explicit instruction to verify against `internal/auth/middleware`, not an unfilled placeholder. All code steps contain complete code.

**Type consistency:** `StyleProfileView`, `UpsertParams`, `UnknownStyleTagsError`, `StyleTagRef` are defined in Task 2 and used identically in Tasks 3–5. Repo interface `StyleProfileRepo` (Load/Upsert/UnknownTagIDs) matches the fakes in Tasks 4–5 and the PG impl in Task 3. `service.New(repo)` / `handler.New(svc)` / `Mount(rg, h)` constructors are consistent with the wiring in Task 6.
