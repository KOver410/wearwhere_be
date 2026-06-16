# UC30 View Smart Wardrobe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A "Smart Wardrobe" for logged-in customers: build a digital closet from delivered purchases, have Gemini compose outfits, and suggest complementary products to buy — useful even with an empty closet (full buy-the-outfit suggestions grounded in the style profile).

**Architecture:** A new provider-agnostic LLM port `internal/shared/llm` (Gemini HTTP adapter + deterministic mock, selected by `AI_PROVIDER`) — built here because nothing else has built it yet. A new `internal/wardrobe` module (domain → repo → service → handler) mounted on `/api/v1/me`. The service loads the closet + style profile, computes a combined `signature` over (closet product ids ⊕ profile), and serves a cached per-user JSONB snapshot unless the signature changed (→ regenerate via Gemini). Gemini receives a list of items and returns outfit groupings; the service maps groupings back to real DB products (owned closet items and/or retriever-sourced products to buy) — the model never invents products. Graceful degrade: on provider failure the closet is still returned with `outfits_status:"unavailable"`.

**Tech Stack:** Go 1.23, gin, pgx/v5 (PostgreSQL JSONB), go-redis (unused here), Google Gemini Generative Language REST API, testify. Pure logic unit-tested with a mock LLM + fakes; repo uses `//go:build integration` + `TEST_DATABASE_URL`; the Gemini HTTP adapter is tested with `httptest.NewServer`.

**Spec:** `docs/superpowers/specs/2026-06-16-ai-personalization-design.md` (§5), as refined: single JSONB snapshot per user, combined closet+profile signature (self-invalidating, no cross-module callback), unified LLM "group these items into outfits" contract.

**Conventions (verified):**
- External client: interface + http adapter (`NewHTTPClient(apiKey, model, baseURL)`, configurable) + mock + `NewFromConfig(Config{Mode,...})` factory — mirrors `internal/shipping/goship`.
- Config: typed sub-struct + `getEnv`/`getDuration` in `Load()` (mirror `GoongConfig`).
- Repo: `DBTX`, `New*PG(db)`. Handlers: `authmw.UserID(c)`, `httpx.OK/ErrorFromApp`. Mounted on the `/me` customer group.
- Catalog retriever for to_buy: `productservice.CatalogService.List(ctx, *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error)` — filter by `Style []string` (slugs), `PriceMin/PriceMax *float64`, `Limit`. `CatalogItem` has Product (ID, Slug, Name, BrandID, Currency...), BrandSlug, BrandName, MinPrice, InStock, PrimaryImage.
- Latest migration: `000045`. Next free: **000046** (bump if taken).
- Redis client `rdb` exists in main.go (not needed by wardrobe).

**File structure:**
```
internal/shared/llm/
  client.go     Client interface + GenerateRequest/GenerateResponse
  mock.go       MockClient (deterministic; canned outfit JSON)
  gemini.go     GeminiClient HTTP adapter (generateContent + usage mapping)
  factory.go    NewFromConfig(Config) (Client, error)
  gemini_test.go (httptest adapter test)
  mock_test.go
internal/config/config.go            MODIFY  add AIConfig
db/migrations/000046_create_wardrobe_snapshots.{up,down}.sql   NEW
internal/wardrobe/
  domain/
    dto.go      ClosetItem, Outfit, OutfitCard, WardrobeResponse, llmOutfit parse structs
  repo/
    repo.go     DBTX, ClosetRepo, SnapshotRepo interfaces
    closet_pg.go     ClosetItems(ctx,userID)
    snapshot_pg.go   Load / Upsert (JSONB)
    wardrobe_pg_test.go (integration)
  service/
    prompt.go   buildItemsPrompt + parseOutfits (pure)
    prompt_test.go
    signature.go  computeSignature (pure)
    signature_test.go
    retriever.go  Retriever interface + CatalogRetriever adapter
    service.go    Service.Get / Regenerate orchestration
    service_test.go (unit, fakes incl mock llm)
  handler/
    handler.go   GET /wardrobe, POST /wardrobe/regenerate
    routes.go    Mount
    handler_test.go
cmd/api/main.go   MODIFY  build llm client, wardrobe module, mount
```

---

## Task 1: Config — AI / Gemini block

**Files:** Modify `internal/config/config.go`

- [ ] **Step 1: Add `AIConfig` and field**

Mirror `GoongConfig`. Add:
```go
type AIConfig struct {
	Provider  string        // env AI_PROVIDER: "mock" | "gemini"  (default "mock")
	APIKey    string        // env GEMINI_API_KEY
	Model     string        // env GEMINI_MODEL (default "gemini-2.0-flash")
	BaseURL   string        // env GEMINI_BASE_URL (default "https://generativelanguage.googleapis.com")
	Timeout   time.Duration // env AI_REQUEST_TIMEOUT (default 30s)
}
```
Add field `AI AIConfig` to `Config`. In `Load()` (mirror the Goong block):
```go
	cfg.AI = AIConfig{
		Provider: getEnv("AI_PROVIDER", "mock"),
		APIKey:   getEnv("GEMINI_API_KEY", ""),
		Model:    getEnv("GEMINI_MODEL", "gemini-2.0-flash"),
		BaseURL:  getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		Timeout:  getDuration("AI_REQUEST_TIMEOUT", 30*time.Second),
	}
```
Confirm `time` is imported (it is, used by other durations).

- [ ] **Step 2: Build** — `go build ./internal/config/... && go build ./...` (success).
- [ ] **Step 3: Commit**
```bash
git add internal/config/config.go
git commit -m "feat(ai): config block for LLM provider (Gemini)"
```

---

## Task 2: LLM port (`internal/shared/llm`)

**Files:** Create `client.go`, `mock.go`, `gemini.go`, `factory.go`, `gemini_test.go`, `mock_test.go`.

- [ ] **Step 1: Write `client.go`**

```go
// Package llm is a provider-agnostic text-generation port. Adapters: gemini
// (HTTP) and mock (deterministic). Select via factory.NewFromConfig.
package llm

import (
	"context"
	"errors"
)

// ErrUnavailable means the provider failed (timeout, non-2xx, decode error,
// or safety block). Callers degrade gracefully rather than surfacing 500s.
var ErrUnavailable = errors.New("llm: provider unavailable")

// GenerateRequest is a single-shot generation: an optional system instruction
// plus the user prompt. (No multi-turn — wardrobe is one-shot.)
type GenerateRequest struct {
	System string
	Prompt string
}

// GenerateResponse is the model output plus token accounting.
type GenerateResponse struct {
	Text      string
	TokensIn  int
	TokensOut int
	Model     string
}

// Client generates text from a prompt.
type Client interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}
```

- [ ] **Step 2: Write `mock.go`**

```go
package llm

import "context"

// MockClient returns a deterministic canned response. Used for dev/test/CI so
// no network or API key is needed (AI_PROVIDER=mock, the default).
type MockClient struct {
	// Response, if set, overrides the default canned text.
	Response string
}

func NewMockClient() *MockClient { return &MockClient{} }

// DefaultMockOutfitJSON is a canned outfit grouping the wardrobe service can
// parse: it references item ids "1" and "2" (the service substitutes real ids
// before calling, so tests set Response explicitly when they need real ids).
const DefaultMockOutfitJSON = `{"outfits":[{"title":"Everyday look","note":"A simple, versatile pairing.","item_ids":["1","2"]}]}`

func (m *MockClient) Generate(_ context.Context, _ GenerateRequest) (*GenerateResponse, error) {
	text := m.Response
	if text == "" {
		text = DefaultMockOutfitJSON
	}
	return &GenerateResponse{Text: text, TokensIn: 0, TokensOut: 0, Model: "mock"}, nil
}
```

- [ ] **Step 3: Write `gemini.go`** (HTTP adapter for the Generative Language REST API)

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GeminiClient calls the Generative Language API generateContent endpoint.
type GeminiClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewGeminiClient builds the adapter. baseURL is configurable so tests can
// point it at an httptest server.
func NewGeminiClient(apiKey, model, baseURL string, timeout time.Duration) *GeminiClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GeminiClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Request/response wire types (subset of the API we use).
type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  *geminiGenCfg   `json:"generationConfig,omitempty"`
}
type geminiGenCfg struct {
	ResponseMIMEType string `json:"responseMimeType,omitempty"`
}
type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (c *GeminiClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	body := geminiRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		// Ask for raw JSON output so the wardrobe parser gets clean JSON.
		GenerationConfig: &geminiGenCfg{ResponseMIMEType: "application/json"},
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrUnavailable, err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: new request: %v", ErrUnavailable, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrUnavailable, resp.StatusCode, string(b))
	}

	var gr geminiResponse
	if err := json.Unmarshal(b, &gr); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("%w: empty candidates (finish reason may be safety)", ErrUnavailable)
	}
	return &GenerateResponse{
		Text:      gr.Candidates[0].Content.Parts[0].Text,
		TokensIn:  gr.UsageMetadata.PromptTokenCount,
		TokensOut: gr.UsageMetadata.CandidatesTokenCount,
		Model:     c.model,
	}, nil
}
```

- [ ] **Step 4: Write `factory.go`**

```go
package llm

import (
	"fmt"
	"time"
)

// Config selects and configures the adapter.
type Config struct {
	Provider string // "mock" | "gemini"
	APIKey   string
	Model    string
	BaseURL  string
	Timeout  time.Duration
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Provider {
	case "mock", "":
		return NewMockClient(), nil
	case "gemini":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("llm: gemini provider requires GEMINI_API_KEY")
		}
		return NewGeminiClient(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Timeout), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (want mock|gemini)", cfg.Provider)
	}
}
```

- [ ] **Step 5: Write `mock_test.go`**

```go
package llm_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestMockClient_DefaultAndOverride(t *testing.T) {
	m := llm.NewMockClient()
	resp, err := m.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.NoError(t, err)
	require.Equal(t, llm.DefaultMockOutfitJSON, resp.Text)
	require.Equal(t, "mock", resp.Model)

	m.Response = `{"outfits":[]}`
	resp, err = m.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.NoError(t, err)
	require.Equal(t, `{"outfits":[]}`, resp.Text)
}

func TestNewFromConfig(t *testing.T) {
	c, err := llm.NewFromConfig(llm.Config{Provider: "mock"})
	require.NoError(t, err)
	require.NotNil(t, c)

	_, err = llm.NewFromConfig(llm.Config{Provider: "gemini"})
	require.Error(t, err, "gemini without API key must error")

	_, err = llm.NewFromConfig(llm.Config{Provider: "bogus"})
	require.Error(t, err)
}
```

- [ ] **Step 6: Write `gemini_test.go`** (httptest fake server)

```go
package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestGeminiClient_GenerateMapsResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, ":generateContent")
		require.Equal(t, "test-key", r.URL.Query().Get("key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"{\"outfits\":[]}"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7}
		}`))
	}))
	defer ts.Close()

	c := llm.NewGeminiClient("test-key", "gemini-2.0-flash", ts.URL, 5*time.Second)
	resp, err := c.Generate(context.Background(), llm.GenerateRequest{System: "sys", Prompt: "hi"})
	require.NoError(t, err)
	require.Equal(t, `{"outfits":[]}`, resp.Text)
	require.Equal(t, 12, resp.TokensIn)
	require.Equal(t, 7, resp.TokensOut)
	require.Equal(t, "gemini-2.0-flash", resp.Model)
}

func TestGeminiClient_ErrorOnNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad"}`, http.StatusInternalServerError)
	}))
	defer ts.Close()
	c := llm.NewGeminiClient("k", "m", ts.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.ErrorIs(t, err, llm.ErrUnavailable)
}

func TestGeminiClient_ErrorOnEmptyCandidates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer ts.Close()
	c := llm.NewGeminiClient("k", "m", ts.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.ErrorIs(t, err, llm.ErrUnavailable)
	require.True(t, strings.Contains(err.Error(), "empty candidates"))
}
```

- [ ] **Step 7: Verify + commit**

Run: `go test ./internal/shared/llm/... -v` (all PASS), `go build ./...`.
```bash
git add internal/shared/llm
git commit -m "feat(llm): provider-agnostic LLM port with gemini + mock adapters"
```

---

## Task 3: Migration — `wardrobe_snapshots`

**Files:** `db/migrations/000046_create_wardrobe_snapshots.{up,down}.sql`

- [ ] **Step 1: Up migration** (verify `000046` is free first; bump if not)

```sql
CREATE TABLE wardrobe_snapshots (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    signature    TEXT NOT NULL,
    outfits      JSONB NOT NULL DEFAULT '[]'::jsonb,
    model        TEXT,
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- [ ] **Step 2: Down migration**
```sql
DROP TABLE IF EXISTS wardrobe_snapshots;
```

- [ ] **Step 3: Apply to the test DB and verify**
```bash
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" up
```
Expected: `46/u create_wardrobe_snapshots` applied. (Also apply to the dev DB `wearwhere` if you run the app: same command with `/wearwhere`.)

- [ ] **Step 4: Commit**
```bash
git add db/migrations/000046_*
git commit -m "feat(wardrobe): wardrobe_snapshots migration"
```

---

## Task 4: Domain types

**Files:** Create `internal/wardrobe/domain/dto.go`

- [ ] **Step 1: Write `dto.go`**

```go
package domain

import "github.com/google/uuid"

// ClosetItem is one owned product (from a delivered purchase) with the
// attributes Gemini needs to reason about pairing.
type ClosetItem struct {
	ProductID    uuid.UUID
	Name         string
	CategorySlug string
	CategoryName string
	StyleSlugs   []string
}

// OutfitCard is a product shown inside an outfit (owned or to-buy).
type OutfitCard struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	BrandSlug    string  `json:"brand_slug"`
	BrandName    string  `json:"brand_name"`
	Currency     string  `json:"currency"`
	MinPrice     float64 `json:"min_price"`
	PrimaryImage *string `json:"primary_image,omitempty"`
}

// Outfit is one composed look: owned pieces plus complementary buys.
type Outfit struct {
	Title string       `json:"title"`
	Note  string       `json:"note"`
	Owned []OutfitCard `json:"owned"`
	ToBuy []OutfitCard `json:"to_buy"`
}

// WardrobeResponse is the GET /me/wardrobe body. ClosetCount lets the FE show
// the closet size without echoing every owned card separately.
type WardrobeResponse struct {
	Closet           []OutfitCard `json:"closet"`
	Outfits          []Outfit     `json:"outfits"`
	OutfitsStatus    string       `json:"outfits_status"` // "ready" | "unavailable"
	OnboardingPrompt bool         `json:"onboarding_prompt"`
}

// LLMOutfit / LLMOutfits are the JSON shape the model returns. item_ids are
// indices into the item list we sent (as strings), which the service maps
// back to real products.
type LLMOutfit struct {
	Title   string   `json:"title"`
	Note    string   `json:"note"`
	ItemIDs []string `json:"item_ids"`
}
type LLMOutfits struct {
	Outfits []LLMOutfit `json:"outfits"`
}
```

- [ ] **Step 2: Build + commit**
```bash
go build ./internal/wardrobe/...
git add internal/wardrobe/domain
git commit -m "feat(wardrobe): domain DTOs"
```

---

## Task 5: Repository (closet query + JSONB snapshot + integration tests)

**Files:** `repo.go`, `closet_pg.go`, `snapshot_pg.go`, `wardrobe_pg_test.go`

- [ ] **Step 1: Write `repo.go`**

```go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

var ErrNoSnapshot = errors.New("wardrobe: no snapshot")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ClosetRepo interface {
	// ClosetItems returns the distinct products a user has received (delivered),
	// each with category + style tag slugs. Empty slice when none.
	ClosetItems(ctx context.Context, userID uuid.UUID) ([]domain.ClosetItem, error)
}

// Snapshot is the persisted feed row.
type Snapshot struct {
	Signature string
	Outfits   []domain.Outfit
}

type SnapshotRepo interface {
	Load(ctx context.Context, userID uuid.UUID) (*Snapshot, error) // ErrNoSnapshot if none
	Upsert(ctx context.Context, userID uuid.UUID, sig string, outfits []domain.Outfit, model string, tokensIn, tokensOut int) error
}
```

- [ ] **Step 2: Write `closet_pg.go`**

```go
package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

type ClosetPG struct{ db DBTX }

func NewClosetPG(db DBTX) *ClosetPG { return &ClosetPG{db: db} }

func (r *ClosetPG) ClosetItems(ctx context.Context, userID uuid.UUID) ([]domain.ClosetItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.name, c.slug, c.name,
		       COALESCE(array_agg(st.slug) FILTER (WHERE st.slug IS NOT NULL), '{}') AS style_slugs
		  FROM order_items oi
		  JOIN sub_orders so ON so.id = oi.sub_order_id
		  JOIN orders o      ON o.id = so.order_id
		  JOIN products p    ON p.id = oi.product_id
		  JOIN categories c  ON c.id = p.category_id
		  LEFT JOIN product_style_tags pst ON pst.product_id = p.id
		  LEFT JOIN style_tags st          ON st.id = pst.style_tag_id
		 WHERE o.user_id = $1 AND so.status = 'delivered' AND p.deleted_at IS NULL
		 GROUP BY p.id, p.name, c.slug, c.name
		 ORDER BY p.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ClosetItem
	for rows.Next() {
		var it domain.ClosetItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.CategorySlug, &it.CategoryName, &it.StyleSlugs); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
```

> **Verify:** `array_agg(st.slug)` where `st.slug` is `CITEXT` scans into `[]string`. pgx decodes a text/citext array into `[]string` fine. If the scan errors on citext, cast in SQL: `array_agg(st.slug::text)`.

- [ ] **Step 3: Write `snapshot_pg.go`**

```go
package repo

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

type SnapshotPG struct{ db DBTX }

func NewSnapshotPG(db DBTX) *SnapshotPG { return &SnapshotPG{db: db} }

func (r *SnapshotPG) Load(ctx context.Context, userID uuid.UUID) (*Snapshot, error) {
	var sig string
	var raw []byte
	err := r.db.QueryRow(ctx,
		`SELECT signature, outfits FROM wardrobe_snapshots WHERE user_id = $1`, userID).
		Scan(&sig, &raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoSnapshot
		}
		return nil, err
	}
	var outfits []domain.Outfit
	if err := json.Unmarshal(raw, &outfits); err != nil {
		return nil, err
	}
	return &Snapshot{Signature: sig, Outfits: outfits}, nil
}

func (r *SnapshotPG) Upsert(ctx context.Context, userID uuid.UUID, sig string, outfits []domain.Outfit, model string, tokensIn, tokensOut int) error {
	raw, err := json.Marshal(outfits)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO wardrobe_snapshots (user_id, signature, outfits, model, tokens_in, tokens_out, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (user_id) DO UPDATE
		  SET signature = EXCLUDED.signature,
		      outfits = EXCLUDED.outfits,
		      model = EXCLUDED.model,
		      tokens_in = EXCLUDED.tokens_in,
		      tokens_out = EXCLUDED.tokens_out,
		      generated_at = NOW()`,
		userID, sig, raw, model, tokensIn, tokensOut)
	return err
}
```

> **Note:** `errors.Is(err, pgx.ErrNoRows)` is the codebase convention; use it instead of `==` (match what `internal/styleprofile/repo` does — it uses `errors.Is`). Add the `errors` import if you switch.

- [ ] **Step 4: Write `wardrobe_pg_test.go`** (integration)

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

	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
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

func TestClosetPG_ReturnsDeliveredProductsWithTags(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	tag := testfixtures.SeedStyleTag(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	variant := testfixtures.SeedVariant(t, tx, prod.ID, "M", "red", 200000, 5)
	_, err := tx.Exec(ctx, `INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1,$2)`, prod.ID, tag.ID)
	require.NoError(t, err)
	testfixtures.SeedDeliveredOrderItem(t, tx, user.ID, brand.ID, prod.ID, variant)

	r := repo.NewClosetPG(tx)
	items, err := r.ClosetItems(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, prod.ID, items[0].ProductID)
	require.Equal(t, cat.Slug, items[0].CategorySlug)
	require.Equal(t, []string{tag.Slug}, items[0].StyleSlugs)
}

func TestClosetPG_EmptyWhenNoDeliveries(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewClosetPG(tx)
	items, err := r.ClosetItems(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestSnapshotPG_UpsertAndLoad(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewSnapshotPG(tx)

	_, err := r.Load(ctx, user.ID)
	require.ErrorIs(t, err, repo.ErrNoSnapshot)

	outfits := []domain.Outfit{{Title: "Look", Note: "n", ToBuy: []domain.OutfitCard{{ID: uuid.New().String(), Name: "X"}}}}
	require.NoError(t, r.Upsert(ctx, user.ID, "sig1", outfits, "mock", 10, 5))

	snap, err := r.Load(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "sig1", snap.Signature)
	require.Len(t, snap.Outfits, 1)
	require.Equal(t, "Look", snap.Outfits[0].Title)

	// Upsert replaces.
	require.NoError(t, r.Upsert(ctx, user.ID, "sig2", nil, "mock", 0, 0))
	snap, err = r.Load(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "sig2", snap.Signature)
	require.Empty(t, snap.Outfits)
}
```

- [ ] **Step 5: Verify + run integration**
```bash
go build ./... && go vet -tags=integration ./internal/wardrobe/repo/...
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -p 1 ./internal/wardrobe/repo/... -v
```
Expected: 3 PASS (requires the 000046 migration applied in Task 3).

- [ ] **Step 6: Commit**
```bash
git add internal/wardrobe/repo
git commit -m "feat(wardrobe): closet query + JSONB snapshot repo"
```

---

## Task 6: Service (prompt, signature, retriever, orchestration)

**Files:** `signature.go`, `signature_test.go`, `prompt.go`, `prompt_test.go`, `retriever.go`, `service.go`, `service_test.go`

- [ ] **Step 1: Write `signature.go` + `signature_test.go`** (pure)

`signature.go`:
```go
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

// ComputeSignature derives a stable fingerprint of the inputs that should
// trigger regeneration: the closet product set and the style profile. When the
// closet is empty the day stamp is folded in so an empty wardrobe refreshes
// daily. Returns a short hex digest.
func ComputeSignature(closet []wdomain.ClosetItem, profile *spdomain.StyleProfileView, dayStamp string) string {
	ids := make([]string, 0, len(closet))
	for _, c := range closet {
		ids = append(ids, c.ProductID.String())
	}
	sort.Strings(ids)

	var prof []string
	if profile != nil {
		for _, t := range profile.StyleTags {
			prof = append(prof, t.ID)
		}
		sort.Strings(prof)
		if profile.BudgetMin != nil {
			prof = append(prof, "bmin:"+strconv.Itoa(*profile.BudgetMin))
		}
		if profile.BudgetMax != nil {
			prof = append(prof, "bmax:"+strconv.Itoa(*profile.BudgetMax))
		}
	}

	var sb strings.Builder
	sb.WriteString("closet:")
	sb.WriteString(strings.Join(ids, ","))
	sb.WriteString("|profile:")
	sb.WriteString(strings.Join(prof, ","))
	if len(closet) == 0 {
		sb.WriteString("|day:")
		sb.WriteString(dayStamp)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:8])
}

var _ = uuid.Nil // keep uuid import if not otherwise used
```
> Remove the `var _ = uuid.Nil` guard and the `uuid` import if unused after writing (it likely is unused — closet ids are stringified via `.String()` on the field; drop the import).

`signature_test.go`:
```go
package service_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

func TestComputeSignature_StableAndSensitive(t *testing.T) {
	c1 := []domain.ClosetItem{{ProductID: uuid.New()}}
	prof := &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String()}}}

	s1 := service.ComputeSignature(c1, prof, "20260616")
	s2 := service.ComputeSignature(c1, prof, "20260616")
	require.Equal(t, s1, s2, "same inputs → same signature")

	// Different closet → different signature.
	c2 := []domain.ClosetItem{{ProductID: uuid.New()}}
	require.NotEqual(t, s1, service.ComputeSignature(c2, prof, "20260616"))

	// Different profile → different signature.
	prof2 := &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String()}}}
	require.NotEqual(t, s1, service.ComputeSignature(c1, prof2, "20260616"))
}

func TestComputeSignature_EmptyClosetVariesByDay(t *testing.T) {
	s1 := service.ComputeSignature(nil, nil, "20260616")
	s2 := service.ComputeSignature(nil, nil, "20260617")
	require.NotEqual(t, s1, s2, "empty closet refreshes daily")
}
```

- [ ] **Step 2: Write `prompt.go` + `prompt_test.go`** (pure)

`prompt.go`:
```go
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

const wardrobeSystemPrompt = `You are a fashion stylist. You will receive a numbered list of clothing items (id, name, category, styles). Group them into 1 to %d cohesive outfits. Each outfit must reference ONLY item ids from the provided list — never invent items. Respond with STRICT JSON: {"outfits":[{"title":string,"note":string,"item_ids":[string,...]}]}. The note is one short sentence of styling advice. Do not include any text outside the JSON.`

// promptItem is one line fed to the model. Index is the stable "id" the model
// echoes back in item_ids.
type promptItem struct {
	ID         string
	Name       string
	Category   string
	StyleSlugs []string
}

// BuildItemsPrompt renders the system + user prompt for a set of items.
func BuildItemsPrompt(items []promptItem, maxOutfits int) (system, user string) {
	system = fmt.Sprintf(wardrobeSystemPrompt, maxOutfits)
	var sb strings.Builder
	sb.WriteString("Items:\n")
	for _, it := range items {
		sb.WriteString(fmt.Sprintf("- id=%s | %s | category=%s | styles=%s\n",
			it.ID, it.Name, it.Category, strings.Join(it.StyleSlugs, ",")))
	}
	return system, sb.String()
}

// ParseOutfits defensively parses the model's JSON. Tolerates code-fence
// wrapping. Returns nil + error on unrecoverable output.
func ParseOutfits(raw string) ([]wdomain.LLMOutfit, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var out wdomain.LLMOutfits
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("wardrobe: parse outfits: %w", err)
	}
	return out.Outfits, nil
}
```

`prompt_test.go`:
```go
package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

func TestParseOutfits_PlainAndFenced(t *testing.T) {
	plain := `{"outfits":[{"title":"A","note":"n","item_ids":["1","2"]}]}`
	got, err := service.ParseOutfits(plain)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"1", "2"}, got[0].ItemIDs)

	fenced := "```json\n" + plain + "\n```"
	got, err = service.ParseOutfits(fenced)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestParseOutfits_GarbageErrors(t *testing.T) {
	_, err := service.ParseOutfits("not json at all")
	require.Error(t, err)
}
```

> `BuildItemsPrompt` and `promptItem` are unexported but referenced in the same package by `service.go`; test only the exported `ParseOutfits` here. (Add a prompt-build test only if you export it.)

- [ ] **Step 3: Write `retriever.go`**

```go
package service

import (
	"context"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

// Retriever fetches active in-stock products to suggest buying, filtered by
// style slugs and budget. Satisfied by CatalogRetriever (adapter over the
// product catalog service).
type Retriever interface {
	Retrieve(ctx context.Context, styleSlugs []string, budgetMin, budgetMax *int, limit int) ([]wdomain.OutfitCard, error)
}

// CatalogLister is the subset of product/service.CatalogService we use.
type CatalogLister interface {
	List(ctx context.Context, q *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error)
}

type CatalogRetriever struct{ catalog CatalogLister }

func NewCatalogRetriever(c CatalogLister) *CatalogRetriever { return &CatalogRetriever{catalog: c} }

func (r *CatalogRetriever) Retrieve(ctx context.Context, styleSlugs []string, budgetMin, budgetMax *int, limit int) ([]wdomain.OutfitCard, error) {
	q := &productdomain.ListProductsQuery{Page: 1, Limit: limit, Sort: "popular"}
	if len(styleSlugs) > 0 {
		// ListProductsQuery caps Style at 10 slugs.
		if len(styleSlugs) > 10 {
			styleSlugs = styleSlugs[:10]
		}
		q.Style = styleSlugs
	}
	if budgetMin != nil {
		v := float64(*budgetMin)
		q.PriceMin = &v
	}
	if budgetMax != nil {
		v := float64(*budgetMax)
		q.PriceMax = &v
	}
	items, _, _, err := r.catalog.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]wdomain.OutfitCard, 0, len(items))
	for _, it := range items {
		out = append(out, catalogItemToCard(it))
	}
	return out, nil
}

func catalogItemToCard(it *productdomain.CatalogItem) wdomain.OutfitCard {
	return wdomain.OutfitCard{
		ID:           it.ID.String(),
		Slug:         it.Slug,
		Name:         it.Name,
		BrandSlug:    it.BrandSlug,
		BrandName:    it.BrandName,
		Currency:     it.Currency,
		MinPrice:     it.MinPrice,
		PrimaryImage: it.PrimaryImage,
	}
}
```

> **Verify** the `CatalogItem` field names against `internal/product/domain/product.go` (it embeds `Product`, so `it.ID`, `it.Slug`, `it.Name`, `it.Currency` come from the embedded struct; `it.BrandSlug/BrandName/MinPrice/PrimaryImage` are on `CatalogItem`). Adjust if a field differs.

- [ ] **Step 4: Write `service_test.go`** (unit, fakes)

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	wrepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

type fakeCloset struct{ items []wdomain.ClosetItem }

func (f *fakeCloset) ClosetItems(_ context.Context, _ uuid.UUID) ([]wdomain.ClosetItem, error) {
	return f.items, nil
}

type fakeSnap struct {
	snap     *wrepo.Snapshot
	upserted bool
}

func (f *fakeSnap) Load(_ context.Context, _ uuid.UUID) (*wrepo.Snapshot, error) {
	if f.snap == nil {
		return nil, wrepo.ErrNoSnapshot
	}
	return f.snap, nil
}
func (f *fakeSnap) Upsert(_ context.Context, _ uuid.UUID, sig string, outfits []wdomain.Outfit, _ string, _, _ int) error {
	f.snap = &wrepo.Snapshot{Signature: sig, Outfits: outfits}
	f.upserted = true
	return nil
}

type fakeProfile struct{ view *spdomain.StyleProfileView }

func (f *fakeProfile) LoadProfile(_ context.Context, _ uuid.UUID) (*spdomain.StyleProfileView, error) {
	return f.view, nil
}

type fakeRetriever struct{ cards []wdomain.OutfitCard }

func (f *fakeRetriever) Retrieve(_ context.Context, _ []string, _, _ *int, _ int) ([]wdomain.OutfitCard, error) {
	return f.cards, nil
}

func cfg() service.Config { return service.Config{MaxOutfits: 5, ToBuyPerOutfit: 2, DayStamp: "20260616"} }

func TestGet_EmptyClosetSuggestsToBuy(t *testing.T) {
	closet := &fakeCloset{}
	snap := &fakeSnap{}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String(), Slug: "minimal"}}}}
	buy := uuid.New().String()
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: buy, Name: "Shirt"}, {ID: uuid.New().String(), Name: "Pant"}}}
	// Mock LLM groups the two retrieved items (ids "1","2") into one outfit.
	mock := llm.NewMockClient()

	svc := service.New(closet, snap, prof, ret, mock, cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, "ready", resp.OutfitsStatus)
	require.NotEmpty(t, resp.Outfits)
	require.Empty(t, resp.Outfits[0].Owned, "empty closet → no owned items")
	require.NotEmpty(t, resp.Outfits[0].ToBuy, "empty closet → all to-buy")
	require.Empty(t, resp.Closet)
	require.False(t, resp.OnboardingPrompt, "profile present → no onboarding prompt")
}

func TestGet_NoProfileNoCloset_OnboardingPrompt(t *testing.T) {
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: uuid.New().String(), Name: "Trend"}, {ID: uuid.New().String(), Name: "Trend2"}}}
	svc := service.New(&fakeCloset{}, &fakeSnap{}, &fakeProfile{}, ret, llm.NewMockClient(), cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.True(t, resp.OnboardingPrompt)
}

func TestGet_ServesCachedSnapshotWhenSignatureMatches(t *testing.T) {
	closet := &fakeCloset{} // empty closet
	prof := &fakeProfile{}
	// Pre-store a snapshot whose signature matches the empty/no-profile/day inputs.
	sig := service.ComputeSignature(nil, nil, "20260616")
	snap := &fakeSnap{snap: &wrepo.Snapshot{Signature: sig, Outfits: []wdomain.Outfit{{Title: "cached"}}}}
	svc := service.New(closet, snap, prof, &fakeRetriever{}, llm.NewMockClient(), cfg())

	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, resp.Outfits, 1)
	require.Equal(t, "cached", resp.Outfits[0].Title)
	require.False(t, snap.upserted, "matching signature must not regenerate")
}

func TestGet_ProviderFailureDegrades(t *testing.T) {
	// closet has an item so closet is returned even when generation fails.
	closet := &fakeCloset{items: []wdomain.ClosetItem{{ProductID: uuid.New(), Name: "Tee"}}}
	failing := &failLLM{}
	svc := service.New(closet, &fakeSnap{}, &fakeProfile{}, &fakeRetriever{}, failing, cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err, "degrade, not error")
	require.Equal(t, "unavailable", resp.OutfitsStatus)
	require.Empty(t, resp.Outfits)
	require.Len(t, resp.Closet, 1, "closet still returned")
}

type failLLM struct{}

func (failLLM) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return nil, llm.ErrUnavailable
}

func TestRegenerate_ForcesUpsert(t *testing.T) {
	sig := service.ComputeSignature(nil, nil, "20260616")
	snap := &fakeSnap{snap: &wrepo.Snapshot{Signature: sig, Outfits: []wdomain.Outfit{{Title: "old"}}}}
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: uuid.New().String(), Name: "A"}, {ID: uuid.New().String(), Name: "B"}}}
	svc := service.New(&fakeCloset{}, snap, &fakeProfile{}, ret, llm.NewMockClient(), cfg())

	_, err := svc.Regenerate(context.Background(), uuid.New())
	require.NoError(t, err)
	require.True(t, snap.upserted, "regenerate always rewrites the snapshot")
}
```

- [ ] **Step 5: Run the tests to verify they fail** (`service.New` etc. undefined): `go test ./internal/wardrobe/service/...` → FAIL.

- [ ] **Step 6: Write `service.go`**

```go
package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	wrepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
)

// Config holds wardrobe tunables.
type Config struct {
	MaxOutfits     int
	ToBuyPerOutfit int
	DayStamp       string // UTC yyyymmdd; injected so the service stays testable
}

// ProfileLoader reads the style profile (styleprofile/service.Service).
type ProfileLoader interface {
	LoadProfile(ctx context.Context, userID uuid.UUID) (*spdomain.StyleProfileView, error)
}

type Service struct {
	closet    wrepo.ClosetRepo
	snapshots wrepo.SnapshotRepo
	profiles  ProfileLoader
	retriever Retriever
	llm       llm.Client
	cfg       Config
}

func New(c wrepo.ClosetRepo, s wrepo.SnapshotRepo, p ProfileLoader, r Retriever, l llm.Client, cfg Config) *Service {
	return &Service{closet: c, snapshots: s, profiles: p, retriever: r, llm: l, cfg: cfg}
}

// Get returns the wardrobe, regenerating only when the signature changed.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*wdomain.WardrobeResponse, error) {
	return s.build(ctx, userID, false)
}

// Regenerate forces a fresh generation regardless of signature.
func (s *Service) Regenerate(ctx context.Context, userID uuid.UUID) (*wdomain.WardrobeResponse, error) {
	return s.build(ctx, userID, true)
}

func (s *Service) build(ctx context.Context, userID uuid.UUID, force bool) (*wdomain.WardrobeResponse, error) {
	closet, err := s.closet.ClosetItems(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.profiles.LoadProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	sig := ComputeSignature(closet, profile, s.cfg.DayStamp)
	onboarding := profile == nil && len(closet) == 0

	closetCards := closetToCards(closet)

	// Serve cached snapshot if fresh (and not forced).
	if !force {
		if snap, err := s.snapshots.Load(ctx, userID); err == nil && snap.Signature == sig {
			return &wdomain.WardrobeResponse{
				Closet: closetCards, Outfits: snap.Outfits,
				OutfitsStatus: "ready", OnboardingPrompt: onboarding,
			}, nil
		} else if err != nil && !errors.Is(err, wrepo.ErrNoSnapshot) {
			return nil, err
		}
	}

	outfits, model, tin, tout, genErr := s.generate(ctx, closet, profile)
	if genErr != nil {
		// Graceful degrade: closet still viewable.
		return &wdomain.WardrobeResponse{
			Closet: closetCards, Outfits: []wdomain.Outfit{},
			OutfitsStatus: "unavailable", OnboardingPrompt: onboarding,
		}, nil
	}

	if err := s.snapshots.Upsert(ctx, userID, sig, outfits, model, tin, tout); err != nil {
		return nil, err
	}
	return &wdomain.WardrobeResponse{
		Closet: closetCards, Outfits: outfits,
		OutfitsStatus: "ready", OnboardingPrompt: onboarding,
	}, nil
}

// generate runs the LLM grouping and assembles outfits. Two modes:
//   - closet non-empty: feed owned items → owned[]; add to_buy via retriever.
//   - closet empty: feed retrieved candidates → to_buy[]; owned stays empty.
func (s *Service) generate(ctx context.Context, closet []wdomain.ClosetItem, profile *spdomain.StyleProfileView) ([]wdomain.Outfit, string, int, int, error) {
	var budgetMin, budgetMax *int
	var profileStyles []string
	if profile != nil {
		budgetMin, budgetMax = profile.BudgetMin, profile.BudgetMax
		for _, t := range profile.StyleTags {
			profileStyles = append(profileStyles, t.Slug)
		}
	}

	if len(closet) > 0 {
		return s.generateFromCloset(ctx, closet, profileStyles, budgetMin, budgetMax)
	}
	return s.generateToBuy(ctx, profileStyles, budgetMin, budgetMax)
}

func (s *Service) generateFromCloset(ctx context.Context, closet []wdomain.ClosetItem, profileStyles []string, bMin, bMax *int) ([]wdomain.Outfit, string, int, int, error) {
	items := make([]promptItem, len(closet))
	byID := make(map[string]wdomain.ClosetItem, len(closet))
	for i, c := range closet {
		id := strconv.Itoa(i + 1)
		items[i] = promptItem{ID: id, Name: c.Name, Category: c.CategoryName, StyleSlugs: c.StyleSlugs}
		byID[id] = c
	}
	llmOutfits, resp, err := s.callLLM(ctx, items)
	if err != nil {
		return nil, "", 0, 0, err
	}

	// to_buy: retrieve complementary products by profile/closet styles.
	styles := profileStyles
	if len(styles) == 0 {
		styles = closetStyleSlugs(closet)
	}
	toBuy, _ := s.retriever.Retrieve(ctx, styles, bMin, bMax, s.cfg.ToBuyPerOutfit)

	var out []wdomain.Outfit
	for _, lo := range llmOutfits {
		var owned []wdomain.OutfitCard
		for _, id := range lo.ItemIDs {
			if c, ok := byID[id]; ok {
				owned = append(owned, closetItemToCard(c))
			}
		}
		if len(owned) == 0 {
			continue // model referenced no real owned items; skip
		}
		out = append(out, wdomain.Outfit{Title: lo.Title, Note: lo.Note, Owned: owned, ToBuy: toBuy})
	}
	return out, resp.Model, resp.TokensIn, resp.TokensOut, nil
}

func (s *Service) generateToBuy(ctx context.Context, profileStyles []string, bMin, bMax *int) ([]wdomain.Outfit, string, int, int, error) {
	// Empty closet: retrieve a candidate set to compose buy-the-outfit looks.
	// Profile styles drive it; with no profile the retriever falls back to
	// popular products (no style filter).
	cands, err := s.retriever.Retrieve(ctx, profileStyles, bMin, bMax, s.cfg.MaxOutfits*s.cfg.ToBuyPerOutfit)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if len(cands) == 0 {
		return []wdomain.Outfit{}, "none", 0, 0, nil
	}
	items := make([]promptItem, len(cands))
	byID := make(map[string]wdomain.OutfitCard, len(cands))
	for i, c := range cands {
		id := strconv.Itoa(i + 1)
		items[i] = promptItem{ID: id, Name: c.Name}
		byID[id] = c
	}
	llmOutfits, resp, err := s.callLLM(ctx, items)
	if err != nil {
		return nil, "", 0, 0, err
	}
	var out []wdomain.Outfit
	for _, lo := range llmOutfits {
		var toBuy []wdomain.OutfitCard
		for _, id := range lo.ItemIDs {
			if c, ok := byID[id]; ok {
				toBuy = append(toBuy, c)
			}
		}
		if len(toBuy) == 0 {
			continue
		}
		out = append(out, wdomain.Outfit{Title: lo.Title, Note: lo.Note, Owned: []wdomain.OutfitCard{}, ToBuy: toBuy})
	}
	return out, resp.Model, resp.TokensIn, resp.TokensOut, nil
}

func (s *Service) callLLM(ctx context.Context, items []promptItem) ([]wdomain.LLMOutfit, *llm.GenerateResponse, error) {
	system, user := BuildItemsPrompt(items, s.cfg.MaxOutfits)
	resp, err := s.llm.Generate(ctx, llm.GenerateRequest{System: system, Prompt: user})
	if err != nil {
		return nil, nil, err
	}
	parsed, err := ParseOutfits(resp.Text)
	if err != nil {
		return nil, nil, err
	}
	return parsed, resp, nil
}

func closetToCards(items []wdomain.ClosetItem) []wdomain.OutfitCard {
	out := make([]wdomain.OutfitCard, 0, len(items))
	for _, c := range items {
		out = append(out, closetItemToCard(c))
	}
	return out
}

func closetItemToCard(c wdomain.ClosetItem) wdomain.OutfitCard {
	return wdomain.OutfitCard{ID: c.ProductID.String(), Name: c.Name}
}

func closetStyleSlugs(items []wdomain.ClosetItem) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range items {
		for _, s := range c.StyleSlugs {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
```

> **Note on closet cards:** the closet query (Task 5) returns id+name+category+styles (enough for the prompt and a minimal card). The card here carries id+name only. If the FE needs full closet cards (slug/brand/image/price), extend `ClosetItems` to select those columns and enrich `closetItemToCard` — out of scope for this plan's MVP; the owned card's id lets the FE hydrate from the catalog. Document this in the response if needed.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/wardrobe/service/... -v`
Expected: all signature, prompt, and service tests PASS.

- [ ] **Step 8: Commit**
```bash
git add internal/wardrobe/service
git commit -m "feat(wardrobe): signature, prompt, retriever, and generation service"
```

---

## Task 7: Handler + routes

**Files:** `handler.go`, `routes.go`, `handler_test.go`

- [ ] **Step 1: Write `handler.go`**

```go
package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// WardrobeService is the capability the handler needs.
type WardrobeService interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.WardrobeResponse, error)
	Regenerate(ctx context.Context, userID uuid.UUID) (*domain.WardrobeResponse, error)
}

type Handler struct{ svc WardrobeService }

func New(svc WardrobeService) *Handler { return &Handler{svc: svc} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func (h *Handler) Get(c *gin.Context) {
	resp, err := h.svc.Get(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Regenerate(c *gin.Context) {
	resp, err := h.svc.Regenerate(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
```

- [ ] **Step 2: Write `routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

// Mount registers wardrobe routes under the /me customer group
// (RequireAuth + RequireRole(customer) already applied).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/wardrobe", h.Get)
	rg.POST("/wardrobe/regenerate", h.Regenerate)
}
```

- [ ] **Step 3: Write `handler_test.go`**

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
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/handler"
)

type fakeSvc struct{ regen bool }

func (f *fakeSvc) Get(_ context.Context, _ uuid.UUID) (*domain.WardrobeResponse, error) {
	return &domain.WardrobeResponse{OutfitsStatus: "ready", Outfits: []domain.Outfit{{Title: "L"}}}, nil
}
func (f *fakeSvc) Regenerate(_ context.Context, _ uuid.UUID) (*domain.WardrobeResponse, error) {
	f.regen = true
	return &domain.WardrobeResponse{OutfitsStatus: "ready"}, nil
}

func setup(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
	handler.Mount(r.Group("/me"), handler.New(f))
	return r
}

func TestGet_ReturnsWardrobe(t *testing.T) {
	r := setup(&fakeSvc{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/wardrobe", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var body domain.WardrobeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "ready", body.OutfitsStatus)
	require.Len(t, body.Outfits, 1)
}

func TestRegenerate_CallsService(t *testing.T) {
	f := &fakeSvc{}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/me/wardrobe/regenerate", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, f.regen)
}
```

- [ ] **Step 4: Run + build** — `go test ./internal/wardrobe/handler/... -v` (2 PASS), `go build ./...`.
- [ ] **Step 5: Commit**
```bash
git add internal/wardrobe/handler
git commit -m "feat(wardrobe): GET /me/wardrobe + POST /me/wardrobe/regenerate"
```

---

## Task 8: Wire into the API

**Files:** Modify `cmd/api/main.go`

- [ ] **Step 1: Build the LLM client once, near the other client/config setup**

Add imports:
```go
	"time"
	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	wardrobehandler "github.com/wearwhere/wearwhere_be/internal/wardrobe/handler"
	wardroberepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
	wardrobeservice "github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
```
(Drop `time` from the import line if already present.)

After config is loaded and other clients are built, add:
```go
	llmClient, err := llm.NewFromConfig(llm.Config{
		Provider: cfg.AI.Provider,
		APIKey:   cfg.AI.APIKey,
		Model:    cfg.AI.Model,
		BaseURL:  cfg.AI.BaseURL,
		Timeout:  cfg.AI.Timeout,
	})
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
```

- [ ] **Step 2: Construct the wardrobe module**

Find the catalog service variable in main.go (the `productservice.CatalogService` instance — search for `NewCatalog(` / `catalogSvc` / `catalogService`; it backs the public catalog routes). Use its real name (shown as `catalogSvc`). It satisfies `wardrobeservice.CatalogLister`. Near the styleprofile/recommendation construction add:

```go
	dayStamp := time.Now().UTC().Format("20060102")
	wardrobeSvc := wardrobeservice.New(
		wardroberepo.NewClosetPG(pgPool),
		wardroberepo.NewSnapshotPG(pgPool),
		styleProfileSvc, // ProfileLoader
		wardrobeservice.NewCatalogRetriever(catalogSvc),
		llmClient,
		wardrobeservice.Config{MaxOutfits: 5, ToBuyPerOutfit: 2, DayStamp: dayStamp},
	)
	wardrobeHandler := wardrobehandler.New(wardrobeSvc)
```

> **`DayStamp` caveat:** it is computed once at startup. For a long-running process this means the empty-closet daily refresh keys off the process start date, not the live date. Acceptable for this project (processes restart on deploy). If you want true daily rollover without restart, change `service.Config.DayStamp` to a `func() string` and call `time.Now()` inside `build` — note this in a follow-up rather than implementing now.

- [ ] **Step 3: Mount on the customer group**

After the recommendation mount in the `customerGroup` block:
```go
	wardrobehandler.Mount(customerGroup, wardrobeHandler)
```

- [ ] **Step 4: Verify**
- `go build ./cmd/api/... && go build ./...` (success)
- `go test ./internal/wardrobe/... ./internal/shared/llm/...` (unit PASS)

- [ ] **Step 5: Commit**
```bash
git add cmd/api/main.go
git commit -m "feat(wardrobe): wire LLM client + wardrobe module into the API"
```

---

## Self-Review

**Spec coverage (§5, as refined):**
- No order minimum; empty closet still useful (full buy-the-outfit) → Task 6 `generateToBuy`. ✓
- Digital closet from delivered purchases with attributes → Task 5 `ClosetItems`. ✓
- Gemini composes outfits; never invents products (maps ids back to DB; skips outfits with no real items) → Task 6 + system prompt. ✓
- Each outfit `owned[]` + `to_buy[]` → Task 4 `Outfit` + Task 6 mapping. ✓
- Empty closet → `to_buy` only, profile-grounded; extreme cold (no profile) → trending via retriever fallback + `onboarding_prompt` → Task 6. ✓
- Persisted snapshot, regenerate on signature change (closet ⊕ profile, daily when empty) → Task 3 + Task 6 `ComputeSignature`/`build`. ✓
- `GET /me/wardrobe` + `POST /me/wardrobe/regenerate` → Task 7. ✓
- Provider failure degrade: closet returned, `outfits_status:"unavailable"` → Task 6 `build`. ✓
- Text metadata only (name/category/styles) to Gemini; no images → Task 6 `promptItem` carries no image. ✓
- Shared `internal/shared/llm` port (gemini + mock), `AI_PROVIDER` default mock → Task 1 + Task 2. ✓
- Token accounting persisted → Task 5 `Upsert(model, tokensIn, tokensOut)`. ✓

**Placeholder scan:** `signature.go` Step 1 includes a `var _ = uuid.Nil` import-guard explicitly flagged for removal — not a real placeholder. The `service.go` closet-card MVP note and the `DayStamp` startup caveat are documented decisions with named follow-ups, not gaps.

**Type consistency:** `ClosetItem`, `Outfit`, `OutfitCard`, `WardrobeResponse`, `LLMOutfit(s)` (Task 4) are used identically in Tasks 5–7. `ClosetRepo`/`SnapshotRepo` + `Snapshot` (Task 5) match the fakes in Task 6. `llm.Client`/`GenerateRequest`/`GenerateResponse`/`ErrUnavailable` (Task 2) are used by Task 6 and the mock. `Retriever`/`CatalogLister` (Task 6) — `CatalogLister.List` matches `productservice.CatalogService.List`'s real signature. `service.New(closet, snap, profile, retriever, llm, cfg)` matches Task 6 tests and Task 8 wiring. `WardrobeService.{Get,Regenerate}` (Task 7) match `Service` methods.

**Follow-ups to file as issues:** (1) full closet cards (slug/brand/image/price) if FE needs them; (2) live daily rollover for empty-closet signature (DayStamp as a func); (3) optional per-user daily regeneration quota (reuse `internal/stylist/service` `RedisQuotaGate`).
