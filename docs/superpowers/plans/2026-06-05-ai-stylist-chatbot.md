# AI Stylist Chatbot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a multi-turn AI stylist chatbot for logged-in customers, grounded in the real WearWhere catalog via a 2-pass RAG-lite pipeline on the Gemini API, with persisted conversations, per-user daily quota, and per-message token accounting.

**Architecture:** New `internal/stylist` module (domain/repo/service/handler) following the existing layered pattern, plus a reusable provider-agnostic `internal/shared/llm` port with `gemini` + `mock` adapters. The chat service orchestrates: quota gate (Redis) → load context → Gemini intent extraction (Pass 1) → existing catalog query (retrieve) → Gemini grounded answer (Pass 2) → persist + attach real product cards. Single-JSON (non-streaming) responses. Errors use Format A (nested `{error:{code,message,details}}`) via `pkg/httpx`.

**Tech Stack:** Go 1.23, gin, pgx/v5 (PostgreSQL), go-redis/v9, Gemini Generative Language REST API. Tests: testify; pure-logic tests run untagged with fakes; DB/E2E tests use `//go:build integration` + `TEST_DATABASE_URL` + `internal/testfixtures`.

**Spec:** `docs/superpowers/specs/2026-06-05-ai-stylist-chatbot-design.md`

**Conventions to follow (verified against the codebase):**
- Errors: services return `*httpx.AppError` (from `pkg/httpx`); handlers call `httpx.ErrorFromApp(c, err)`. Success: `httpx.OK` / `httpx.Created` / `httpx.NoContent`.
- Auth: handlers read the caller via `authmw.UserID(c)` (import `authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"`). Routes mounted on a group that already applied `RequireAuth` + `RequireRole(customer)`.
- External clients: interface + `mock`/http adapters + `NewFromConfig` factory selecting by a `Mode`/`Provider` string (see `internal/shipping/goship`).
- Repos: `DBTX` interface; `New<Thing>PG(db)` constructors; sentinel `repo.ErrNotFound` mapped to a domain `AppError` in the service.
- Config: add a typed sub-struct to `config.Config`, populate in `Load()` with `getEnv/getInt/getDuration`.
- Time formatting in responses: `time.Time.UTC().Format(time.RFC3339)`.

**Implementation order:** Phase 0 (shared foundations) → Phase 1 (migrations) → Phase 2 (domain) → Phase 3 (repo) → Phase 4 (service) → Phase 5 (handler) → Phase 6 (wiring + E2E).

---

## Phase 0 — Shared foundations

### Task 1: Add optional `Details` to `httpx.AppError`

The quota error must surface `{limit, used}` in `error.details`. `AppError` currently has no details field. Add one (additive, backward-compatible — all existing call sites keep working).

**Files:**
- Modify: `pkg/httpx/response.go`
- Test: `pkg/httpx/response_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `pkg/httpx/response_test.go`:

```go
package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func TestErrorFromApp_IncludesDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := httpx.NewAppErrorWithDetails(
		http.StatusTooManyRequests, "AI_QUOTA_EXCEEDED", "limit reached",
		map[string]any{"limit": 30, "used": 30},
	)
	httpx.ErrorFromApp(c, err)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.JSONEq(t, `{"error":{"code":"AI_QUOTA_EXCEEDED","message":"limit reached","details":{"limit":30,"used":30}}}`, w.Body.String())
}

func TestErrorFromApp_NoDetailsOmitsKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	httpx.ErrorFromApp(c, httpx.NewAppError(http.StatusNotFound, "X", "nope"))

	require.JSONEq(t, `{"error":{"code":"X","message":"nope"}}`, w.Body.String())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/httpx/ -run TestErrorFromApp -v`
Expected: FAIL — `NewAppErrorWithDetails` undefined.

- [ ] **Step 3: Add `Details` field, constructor, and detail-aware translation**

In `pkg/httpx/response.go`, replace the `AppError` block (struct + `NewAppError` + `ErrorFromApp`) with:

```go
// AppError is a tagged error services return to handlers; handlers translate it
// to the appropriate HTTP status via ErrorFromApp.
type AppError struct {
	Code    string
	Message string
	Status  int
	Details map[string]any
}

func (e *AppError) Error() string { return e.Message }

func NewAppError(status int, code, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

func NewAppErrorWithDetails(status int, code, msg string, details map[string]any) *AppError {
	return &AppError{Status: status, Code: code, Message: msg, Details: details}
}

func ErrorFromApp(c *gin.Context, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		if len(ae.Details) > 0 {
			ErrorWithDetails(c, ae.Status, ae.Code, ae.Message, ae.Details)
			return
		}
		Error(c, ae.Status, ae.Code, ae.Message)
		return
	}
	Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/httpx/ -run TestErrorFromApp -v`
Expected: PASS (both cases).

- [ ] **Step 5: Verify nothing else broke**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/httpx/response.go pkg/httpx/response_test.go
git commit -m "feat(httpx): optional Details on AppError for richer error envelopes"
```

---

### Task 2: AI config block

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestLoad_AIDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "s")
	// ensure no AI_* leak from the host env
	for _, k := range []string{"AI_PROVIDER", "GEMINI_MODEL", "AI_CHAT_DAILY_MESSAGE_LIMIT", "AI_CHAT_MAX_CONTEXT_MESSAGES", "AI_CHAT_RETRIEVE_K", "AI_REQUEST_TIMEOUT"} {
		t.Setenv(k, "")
	}

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "mock", cfg.AI.Provider)
	require.Equal(t, "gemini-2.0-flash", cfg.AI.GeminiModel)
	require.Equal(t, 30, cfg.AI.DailyMessageLimit)
	require.Equal(t, 10, cfg.AI.MaxContextMessages)
	require.Equal(t, 6, cfg.AI.RetrieveK)
	require.Equal(t, 15*time.Second, cfg.AI.RequestTimeout)
}
```

(If `config_test.go` lacks imports for `time`/`require`/`config`, add them — match the existing test file's import style.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_AIDefaults -v`
Expected: FAIL — `cfg.AI` undefined.

- [ ] **Step 3: Add the config struct + wiring**

In `internal/config/config.go`, add `AI AIConfig` to the `Config` struct (after `CORS CORSConfig`):

```go
	CORS        CORSConfig
	AI          AIConfig
```

Add the type (near the other config types, e.g. after `CORSConfig`):

```go
type AIConfig struct {
	Provider           string // gemini | mock
	GeminiAPIKey       string
	GeminiModel        string
	DailyMessageLimit  int
	MaxContextMessages int
	RetrieveK          int
	RequestTimeout     time.Duration
}
```

In `Load()`, after the `cfg.CORS = ...` assignment and before `return cfg, nil`:

```go
	cfg.AI = AIConfig{
		Provider:           getEnv("AI_PROVIDER", "mock"),
		GeminiAPIKey:       getEnv("GEMINI_API_KEY", ""),
		GeminiModel:        getEnv("GEMINI_MODEL", "gemini-2.0-flash"),
		DailyMessageLimit:  getInt("AI_CHAT_DAILY_MESSAGE_LIMIT", 30),
		MaxContextMessages: getInt("AI_CHAT_MAX_CONTEXT_MESSAGES", 10),
		RetrieveK:          getInt("AI_CHAT_RETRIEVE_K", 6),
		RequestTimeout:     getDuration("AI_REQUEST_TIMEOUT", 15*time.Second),
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_AIDefaults -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): AI/Gemini config block (provider, model, limits, timeout)"
```

---

### Task 3: LLM port (types + interface)

**Files:**
- Create: `internal/shared/llm/client.go`

- [ ] **Step 1: Write the port**

Create `internal/shared/llm/client.go`:

```go
// Package llm is a provider-agnostic port for text generation, with gemini and
// mock adapters. It is intentionally minimal: a single Generate call that takes
// a system instruction + a turn history and returns text plus token usage.
package llm

import (
	"context"
	"errors"
)

// ErrUnavailable signals the provider failed (timeout, non-2xx, safety block, or
// empty candidate). Callers map this to a user-facing "try again" error.
var ErrUnavailable = errors.New("llm: provider unavailable")

type Role string

const (
	RoleUser  Role = "user"  // the human turn
	RoleModel Role = "model" // a prior assistant turn
)

type Message struct {
	Role    Role
	Content string
}

type GenerateRequest struct {
	System      string    // system instruction (may be empty)
	Messages    []Message // ordered oldest -> newest; last is the current user turn
	Temperature float32
	MaxTokens   int  // 0 => provider default
	JSON        bool // request strict JSON output (response_mime_type=application/json)
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type GenerateResponse struct {
	Text  string
	Usage Usage
	Model string
}

type Client interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/shared/llm/`
Expected: builds clean (no test yet; covered by Task 4/5).

- [ ] **Step 3: Commit**

```bash
git add internal/shared/llm/client.go
git commit -m "feat(llm): provider-agnostic generation port"
```

---

### Task 4: Mock LLM adapter

**Files:**
- Create: `internal/shared/llm/mock.go`
- Test: `internal/shared/llm/mock_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/shared/llm/mock_test.go`:

```go
package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestMockClient_JSONModeReturnsIntent(t *testing.T) {
	m := llm.NewMockClient()
	resp, err := m.Generate(context.Background(), llm.GenerateRequest{JSON: true})
	require.NoError(t, err)
	require.JSONEq(t, `{"keywords":"váy"}`, resp.Text)
	require.Equal(t, "mock", resp.Model)
	require.Positive(t, resp.Usage.OutputTokens)
}

func TestMockClient_TextModeReturnsAnswer(t *testing.T) {
	m := llm.NewMockClient()
	m.AnswerText = "Gợi ý cho bạn"
	resp, err := m.Generate(context.Background(), llm.GenerateRequest{})
	require.NoError(t, err)
	require.Equal(t, "Gợi ý cho bạn", resp.Text)
}

func TestMockClient_ErrPropagates(t *testing.T) {
	m := llm.NewMockClient()
	m.Err = errors.New("boom")
	_, err := m.Generate(context.Background(), llm.GenerateRequest{})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/llm/ -run TestMockClient -v`
Expected: FAIL — `NewMockClient` undefined.

- [ ] **Step 3: Implement the mock**

Create `internal/shared/llm/mock.go`:

```go
package llm

import "context"

// MockClient returns deterministic canned responses. JSON requests (Pass 1 intent
// extraction) return IntentJSON; all other requests return AnswerText. Tests may
// override any field, including Err to simulate provider failure.
type MockClient struct {
	IntentJSON string
	AnswerText string
	Model      string
	Err        error
}

func NewMockClient() *MockClient {
	return &MockClient{
		IntentJSON: `{"keywords":"váy"}`,
		AnswerText: "Đây là một vài gợi ý phối đồ cho bạn.",
		Model:      "mock",
	}
}

func (m *MockClient) Generate(_ context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	text := m.AnswerText
	if req.JSON {
		text = m.IntentJSON
	}
	return &GenerateResponse{
		Text:  text,
		Usage: Usage{InputTokens: 10, OutputTokens: 20},
		Model: m.Model,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shared/llm/ -run TestMockClient -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/shared/llm/mock.go internal/shared/llm/mock_test.go
git commit -m "feat(llm): deterministic mock adapter"
```

---

### Task 5: Gemini HTTP adapter

Maps our `GenerateRequest` to the Generative Language REST API
(`POST {baseURL}/v1beta/models/{model}:generateContent?key={apiKey}`) and back.

**Files:**
- Create: `internal/shared/llm/gemini.go`
- Test: `internal/shared/llm/gemini_test.go`

- [ ] **Step 1: Write the failing test (httptest, no network)**

Create `internal/shared/llm/gemini_test.go`:

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

func TestGeminiClient_Generate_MapsRequestAndResponse(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"hello there"}]}}],
			"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7}
		}`))
	}))
	defer srv.Close()

	c := llm.NewGeminiClient("test-key", "gemini-2.0-flash", srv.URL, 5*time.Second)
	resp, err := c.Generate(context.Background(), llm.GenerateRequest{
		System:   "be helpful",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		JSON:     true,
	})
	require.NoError(t, err)
	require.Equal(t, "hello there", resp.Text)
	require.Equal(t, 12, resp.Usage.InputTokens)
	require.Equal(t, 7, resp.Usage.OutputTokens)
	require.Equal(t, "gemini-2.0-flash", resp.Model)

	require.Contains(t, gotPath, "/v1beta/models/gemini-2.0-flash:generateContent")
	require.Contains(t, gotPath, "key=test-key")
	require.Contains(t, gotBody, `"system_instruction"`)
	require.Contains(t, gotBody, `"response_mime_type":"application/json"`)
	require.Contains(t, gotBody, `"role":"user"`)
}

func TestGeminiClient_Generate_Non2xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	}))
	defer srv.Close()

	c := llm.NewGeminiClient("k", "m", srv.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "x"}}})
	require.ErrorIs(t, err, llm.ErrUnavailable)
}

func TestGeminiClient_Generate_EmptyCandidatesIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	c := llm.NewGeminiClient("k", "m", srv.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{})
	require.ErrorIs(t, err, llm.ErrUnavailable)
	_ = strings.TrimSpace("") // keep strings import if unused elsewhere
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/llm/ -run TestGeminiClient -v`
Expected: FAIL — `NewGeminiClient` undefined.

- [ ] **Step 3: Implement the adapter**

Create `internal/shared/llm/gemini.go`:

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

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

type GeminiClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewGeminiClient(apiKey, model, baseURL string, timeout time.Duration) *GeminiClient {
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &GeminiClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// ── wire types (subset of the Generative Language API) ──

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiGenConfig struct {
	Temperature      float32 `json:"temperature,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	ResponseMIMEType string  `json:"response_mime_type,omitempty"`
}
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"system_instruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  geminiGenConfig `json:"generationConfig"`
}
type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (c *GeminiClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	body := geminiRequest{
		Contents:         make([]geminiContent, 0, len(req.Messages)),
		GenerationConfig: geminiGenConfig{Temperature: req.Temperature, MaxOutputTokens: req.MaxTokens},
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	if req.JSON {
		body.GenerationConfig.ResponseMIMEType = "application/json"
	}
	for _, m := range req.Messages {
		role := "user"
		if m.Role == RoleModel {
			role = "model"
		}
		body.Contents = append(body.Contents, geminiContent{Role: role, Parts: []geminiPart{{Text: m.Content}}})
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrUnavailable, err)
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("%w: new request: %v", ErrUnavailable, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrUnavailable, resp.StatusCode, string(raw))
	}

	var gr geminiResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("%w: empty candidates", ErrUnavailable)
	}
	return &GenerateResponse{
		Text:  gr.Candidates[0].Content.Parts[0].Text,
		Usage: Usage{InputTokens: gr.UsageMetadata.PromptTokenCount, OutputTokens: gr.UsageMetadata.CandidatesTokenCount},
		Model: c.model,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shared/llm/ -run TestGeminiClient -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/shared/llm/gemini.go internal/shared/llm/gemini_test.go
git commit -m "feat(llm): Gemini generateContent HTTP adapter"
```

---

### Task 6: LLM factory

**Files:**
- Create: `internal/shared/llm/factory.go`
- Test: `internal/shared/llm/factory_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/shared/llm/factory_test.go`:

```go
package llm_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestNewFromConfig(t *testing.T) {
	c, err := llm.NewFromConfig(llm.Config{Provider: "mock"})
	require.NoError(t, err)
	require.IsType(t, &llm.MockClient{}, c)

	c, err = llm.NewFromConfig(llm.Config{Provider: "", APIKey: ""})
	require.NoError(t, err)
	require.IsType(t, &llm.MockClient{}, c)

	c, err = llm.NewFromConfig(llm.Config{Provider: "gemini", APIKey: "k", Model: "m", Timeout: time.Second})
	require.NoError(t, err)
	require.IsType(t, &llm.GeminiClient{}, c)

	_, err = llm.NewFromConfig(llm.Config{Provider: "gemini"})
	require.Error(t, err) // missing API key

	_, err = llm.NewFromConfig(llm.Config{Provider: "bogus"})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/llm/ -run TestNewFromConfig -v`
Expected: FAIL — `NewFromConfig`/`Config` undefined.

- [ ] **Step 3: Implement the factory**

Create `internal/shared/llm/factory.go`:

```go
package llm

import (
	"fmt"
	"time"
)

type Config struct {
	Provider string // gemini | mock
	APIKey   string
	Model    string
	BaseURL  string // optional override (tests); empty => Gemini default
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
		return nil, fmt.Errorf("llm: unknown provider %q (want gemini|mock)", cfg.Provider)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shared/llm/ -v`
Expected: PASS (whole package).

- [ ] **Step 5: Commit**

```bash
git add internal/shared/llm/factory.go internal/shared/llm/factory_test.go
git commit -m "feat(llm): factory selecting gemini|mock by config"
```

---

## Phase 1 — Migrations

### Task 7: `ai_conversations` table

**Files:**
- Create: `db/migrations/000032_create_ai_conversations.up.sql`
- Create: `db/migrations/000032_create_ai_conversations.down.sql`

- [ ] **Step 1: Write the up migration**

Create `db/migrations/000032_create_ai_conversations.up.sql`:

```sql
-- db/migrations/000032_create_ai_conversations.up.sql
CREATE TABLE ai_conversations (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_at  TIMESTAMPTZ,
    archived_at      TIMESTAMPTZ
);

CREATE INDEX ai_conversations_user_idx
    ON ai_conversations (user_id, last_message_at DESC NULLS LAST)
    WHERE archived_at IS NULL;
```

- [ ] **Step 2: Write the down migration**

Create `db/migrations/000032_create_ai_conversations.down.sql`:

```sql
-- db/migrations/000032_create_ai_conversations.down.sql
DROP TABLE IF EXISTS ai_conversations;
```

- [ ] **Step 3: Apply against the test DB to verify it parses**

Run (PowerShell; adjust DSN to your local test DB):
```
$env:TEST_DATABASE_URL = "postgres://postgres:postgres@localhost:5432/wearwhere_test?sslmode=disable"
migrate -path db/migrations -database $env:TEST_DATABASE_URL up
```
Expected: migration `32` applies with no error. (If the project uses a different migration runner/Make target, use that instead — check the repo's existing migration command.)

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000032_create_ai_conversations.up.sql db/migrations/000032_create_ai_conversations.down.sql
git commit -m "feat(db): ai_conversations table"
```

---

### Task 8: `ai_messages` table

**Files:**
- Create: `db/migrations/000033_create_ai_messages.up.sql`
- Create: `db/migrations/000033_create_ai_messages.down.sql`

- [ ] **Step 1: Write the up migration**

Create `db/migrations/000033_create_ai_messages.up.sql`:

```sql
-- db/migrations/000033_create_ai_messages.up.sql
CREATE TABLE ai_messages (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id    UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role               TEXT NOT NULL CHECK (role IN ('user','assistant')),
    content            TEXT NOT NULL,
    cited_product_ids  UUID[] NOT NULL DEFAULT '{}',
    tokens_in          INT  NOT NULL DEFAULT 0,
    tokens_out         INT  NOT NULL DEFAULT 0,
    model              TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ai_messages_conversation_idx
    ON ai_messages (conversation_id, created_at);
```

- [ ] **Step 2: Write the down migration**

Create `db/migrations/000033_create_ai_messages.down.sql`:

```sql
-- db/migrations/000033_create_ai_messages.down.sql
DROP TABLE IF EXISTS ai_messages;
```

- [ ] **Step 3: Apply + roll back to verify both directions**

Run:
```
migrate -path db/migrations -database $env:TEST_DATABASE_URL up
migrate -path db/migrations -database $env:TEST_DATABASE_URL down 1
migrate -path db/migrations -database $env:TEST_DATABASE_URL up
```
Expected: `33` applies, the `down 1` drops `ai_messages`, and re-`up` recreates it — all without error.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000033_create_ai_messages.up.sql db/migrations/000033_create_ai_messages.down.sql
git commit -m "feat(db): ai_messages table"
```

---

## Phase 2 — Domain

### Task 9: Domain entities

**Files:**
- Create: `internal/stylist/domain/conversation.go`

- [ ] **Step 1: Write the entities**

Create `internal/stylist/domain/conversation.go`:

```go
// Package domain holds the stylist chatbot entities and DTOs.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Conversation struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastMessageAt *time.Time
	ArchivedAt    *time.Time
}

type Message struct {
	ID              uuid.UUID
	ConversationID  uuid.UUID
	Role            Role
	Content         string
	CitedProductIDs []uuid.UUID
	TokensIn        int
	TokensOut       int
	Model           *string
	CreatedAt       time.Time
}

// ProductCard is a denormalized product reference attached to an assistant
// message. Built from our own catalog, never from model output.
type ProductCard struct {
	ID              uuid.UUID
	Slug            string
	Name            string
	BrandSlug       string
	BrandName       string
	PrimaryImageURL *string
	PriceVND        int64
}

// StyleProfile is a forward-compat placeholder for B1 (UC31 Set Style
// Preferences). The prompt builder personalizes when this is non-nil; today the
// chat service always passes nil.
type StyleProfile struct {
	Styles    []string
	BudgetMin *int64
	BudgetMax *int64
	Sizes     []string
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/stylist/domain/`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/stylist/domain/conversation.go
git commit -m "feat(stylist): domain entities"
```

---

### Task 10: Domain errors + quota error helper

**Files:**
- Create: `internal/stylist/domain/errors.go`

- [ ] **Step 1: Write the errors**

Create `internal/stylist/domain/errors.go`:

```go
package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
	ErrConversationNotFound = httpx.NewAppError(http.StatusNotFound, "CONVERSATION_NOT_FOUND", "Conversation not found")
	ErrProviderUnavailable  = httpx.NewAppError(http.StatusBadGateway, "AI_PROVIDER_UNAVAILABLE", "AI service is temporarily unavailable")
)

// QuotaExceeded builds a 429 AppError carrying the limit/used counts in details.
func QuotaExceeded(limit, used int) *httpx.AppError {
	return httpx.NewAppErrorWithDetails(
		http.StatusTooManyRequests,
		"AI_QUOTA_EXCEEDED",
		"Daily AI message limit reached",
		map[string]any{"limit": limit, "used": used},
	)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/stylist/domain/`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/stylist/domain/errors.go
git commit -m "feat(stylist): domain errors + quota-exceeded helper"
```

---

### Task 11: Domain DTOs + converters

**Files:**
- Create: `internal/stylist/domain/dto.go`
- Test: `internal/stylist/domain/dto_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/stylist/domain/dto_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

func TestToMessageResponse_AssistantWithCards(t *testing.T) {
	img := "https://img/x.jpg"
	model := "gemini-2.0-flash"
	msg := &domain.Message{
		ID:        uuid.New(),
		Role:      domain.RoleAssistant,
		Content:   "try this",
		Model:     &model,
		CreatedAt: time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
	}
	cards := []domain.ProductCard{{
		ID: uuid.New(), Slug: "tee", Name: "Tee",
		BrandSlug: "rep", BrandName: "REP", PrimaryImageURL: &img, PriceVND: 350000,
	}}

	out := domain.ToMessageResponse(msg, cards)
	require.Equal(t, "assistant", out.Role)
	require.Equal(t, "try this", out.Content)
	require.Equal(t, "2026-06-05T10:00:00Z", out.CreatedAt)
	require.Len(t, out.Products, 1)
	require.Equal(t, "rep", out.Products[0].Brand.Slug)
	require.EqualValues(t, 350000, out.Products[0].PriceVND)
}

func TestToConversationResponse_NilLastMessage(t *testing.T) {
	c := &domain.Conversation{
		ID: uuid.New(), Title: "Hi",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	out := domain.ToConversationResponse(c)
	require.Equal(t, "Hi", out.Title)
	require.Nil(t, out.LastMessageAt)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stylist/domain/ -run TestTo -v`
Expected: FAIL — converters undefined.

- [ ] **Step 3: Implement DTOs + converters**

Create `internal/stylist/domain/dto.go`:

```go
package domain

import "time"

// ── requests ──

type CreateConversationRequest struct {
	FirstMessage string `json:"first_message" binding:"omitempty,max=2000"`
}

type SendMessageRequest struct {
	Content string `json:"content" binding:"required,max=2000"`
}

type RenameConversationRequest struct {
	Title string `json:"title" binding:"required,max=120"`
}

// ── responses ──

type BrandRef struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type ProductCardResponse struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	Brand           BrandRef `json:"brand"`
	PrimaryImageURL *string  `json:"primary_image_url,omitempty"`
	PriceVND        int64    `json:"price_vnd"`
}

type MessageResponse struct {
	ID        string                `json:"id"`
	Role      string                `json:"role"`
	Content   string                `json:"content"`
	Products  []ProductCardResponse `json:"products,omitempty"`
	CreatedAt string                `json:"created_at"`
}

type ConversationResponse struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	LastMessageAt *string `json:"last_message_at,omitempty"`
}

type ConversationDetailResponse struct {
	ConversationResponse
	Messages []MessageResponse `json:"messages"`
}

type QuotaResponse struct {
	Used      int `json:"used"`
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

type SendMessageResponse struct {
	UserMessage      MessageResponse `json:"user_message"`
	AssistantMessage MessageResponse `json:"assistant_message"`
	Quota            QuotaResponse   `json:"quota"`
}

// ── converters ──

func ToProductCardResponse(p ProductCard) ProductCardResponse {
	return ProductCardResponse{
		ID:              p.ID.String(),
		Slug:            p.Slug,
		Name:            p.Name,
		Brand:           BrandRef{Slug: p.BrandSlug, Name: p.BrandName},
		PrimaryImageURL: p.PrimaryImageURL,
		PriceVND:        p.PriceVND,
	}
}

func ToMessageResponse(m *Message, cards []ProductCard) MessageResponse {
	out := MessageResponse{
		ID:        m.ID.String(),
		Role:      string(m.Role),
		Content:   m.Content,
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
	}
	for _, c := range cards {
		out.Products = append(out.Products, ToProductCardResponse(c))
	}
	return out
}

func ToConversationResponse(c *Conversation) ConversationResponse {
	out := ConversationResponse{
		ID:        c.ID.String(),
		Title:     c.Title,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if c.LastMessageAt != nil {
		s := c.LastMessageAt.UTC().Format(time.RFC3339)
		out.LastMessageAt = &s
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stylist/domain/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stylist/domain/dto.go internal/stylist/domain/dto_test.go
git commit -m "feat(stylist): DTOs + response converters"
```

---

## Phase 3 — Repo

### Task 12: Repo interfaces

**Files:**
- Create: `internal/stylist/repo/repo.go`

- [ ] **Step 1: Write the interfaces**

Create `internal/stylist/repo/repo.go`:

```go
// Package repo provides persistence for stylist conversations and messages.
package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

var ErrNotFound = errors.New("stylist: not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ConversationRepo interface {
	Create(ctx context.Context, c *domain.Conversation) error
	FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.Conversation, error)
	List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Conversation, int, error)
	// Touch sets last_message_at and updated_at to ts and overwrites title.
	Touch(ctx context.Context, id uuid.UUID, ts time.Time, title string) error
	Rename(ctx context.Context, id, userID uuid.UUID, title string) (*domain.Conversation, error)
	Archive(ctx context.Context, id, userID uuid.UUID) error
}

type MessageRepo interface {
	Insert(ctx context.Context, m *domain.Message) error
	ListByConversation(ctx context.Context, conversationID uuid.UUID) ([]*domain.Message, error)
	// ListRecent returns the most recent `limit` messages, ordered oldest -> newest.
	ListRecent(ctx context.Context, conversationID uuid.UUID, limit int) ([]*domain.Message, error)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/stylist/repo/`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/stylist/repo/repo.go
git commit -m "feat(stylist): repo interfaces"
```

---

### Task 13: Conversation Postgres repo (integration-tested)

**Files:**
- Create: `internal/stylist/repo/conversation_pg.go`
- Test: `internal/stylist/repo/conversation_pg_test.go`

- [ ] **Step 1: Write the failing integration test**

Create `internal/stylist/repo/conversation_pg_test.go`:

```go
//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required for integration tests")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func TestConversationPG_CreateFindListArchive(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	other := testfixtures.SeedCustomer(t, tx)
	r := repo.NewConversationPG(tx)

	c := &domain.Conversation{UserID: user.ID, Title: "First"}
	require.NoError(t, r.Create(ctx, c))
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", c.ID.String())

	got, err := r.FindByID(ctx, c.ID, user.ID)
	require.NoError(t, err)
	require.Equal(t, "First", got.Title)

	// IDOR: another user cannot read it
	_, err = r.FindByID(ctx, c.ID, other.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Touch updates last_message_at + title
	ts := time.Now().UTC()
	require.NoError(t, r.Touch(ctx, c.ID, ts, "Renamed by touch"))
	got, _ = r.FindByID(ctx, c.ID, user.ID)
	require.NotNil(t, got.LastMessageAt)
	require.Equal(t, "Renamed by touch", got.Title)

	// List excludes other users + archived
	list, total, err := r.List(ctx, user.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)

	require.NoError(t, r.Archive(ctx, c.ID, user.ID))
	_, total, _ = r.List(ctx, user.ID, 20, 0)
	require.Equal(t, 0, total)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./internal/stylist/repo/ -run TestConversationPG -v`
Expected: FAIL — `NewConversationPG` undefined. (Requires `TEST_DATABASE_URL` set to a DB migrated through `000033`.)

- [ ] **Step 3: Implement the repo**

Create `internal/stylist/repo/conversation_pg.go`:

```go
package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

type ConversationPG struct{ db DBTX }

func NewConversationPG(db DBTX) *ConversationPG { return &ConversationPG{db: db} }

func (r *ConversationPG) Create(ctx context.Context, c *domain.Conversation) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO ai_conversations (user_id, title)
		 VALUES ($1, $2)
		 RETURNING id, created_at, updated_at`,
		c.UserID, c.Title,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *ConversationPG) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.Conversation, error) {
	c := &domain.Conversation{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, title, created_at, updated_at, last_message_at, archived_at
		   FROM ai_conversations
		  WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`,
		id, userID,
	).Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt, &c.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ConversationPG) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Conversation, int, error) {
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_conversations WHERE user_id = $1 AND archived_at IS NULL`,
		userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, title, created_at, updated_at, last_message_at, archived_at
		   FROM ai_conversations
		  WHERE user_id = $1 AND archived_at IS NULL
		  ORDER BY last_message_at DESC NULLS LAST, created_at DESC
		  LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.Conversation
	for rows.Next() {
		c := &domain.Conversation{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt, &c.ArchivedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *ConversationPG) Touch(ctx context.Context, id uuid.UUID, ts time.Time, title string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE ai_conversations
		    SET last_message_at = $2, updated_at = $2, title = $3
		  WHERE id = $1`,
		id, ts, title,
	)
	return err
}

func (r *ConversationPG) Rename(ctx context.Context, id, userID uuid.UUID, title string) (*domain.Conversation, error) {
	ct, err := r.db.Exec(ctx,
		`UPDATE ai_conversations SET title = $3, updated_at = NOW()
		  WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`,
		id, userID, title,
	)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.FindByID(ctx, id, userID)
}

func (r *ConversationPG) Archive(ctx context.Context, id, userID uuid.UUID) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE ai_conversations SET archived_at = NOW(), updated_at = NOW()
		  WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags integration ./internal/stylist/repo/ -run TestConversationPG -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stylist/repo/conversation_pg.go internal/stylist/repo/conversation_pg_test.go
git commit -m "feat(stylist): conversation Postgres repo"
```

---

### Task 14: Message Postgres repo (integration-tested)

**Files:**
- Create: `internal/stylist/repo/message_pg.go`
- Test: `internal/stylist/repo/message_pg_test.go`

- [ ] **Step 1: Write the failing integration test**

Create `internal/stylist/repo/message_pg_test.go`:

```go
//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestMessagePG_InsertAndList(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	cr := repo.NewConversationPG(tx)
	mr := repo.NewMessagePG(tx)

	c := &domain.Conversation{UserID: user.ID, Title: "t"}
	require.NoError(t, cr.Create(ctx, c))

	model := "gemini-2.0-flash"
	pid := uuid.New()
	require.NoError(t, mr.Insert(ctx, &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Content: "q1"}))
	require.NoError(t, mr.Insert(ctx, &domain.Message{
		ConversationID: c.ID, Role: domain.RoleAssistant, Content: "a1",
		CitedProductIDs: []uuid.UUID{pid}, TokensIn: 12, TokensOut: 7, Model: &model,
	}))
	require.NoError(t, mr.Insert(ctx, &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Content: "q2"}))

	all, err := mr.ListByConversation(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "q1", all[0].Content) // oldest first
	require.Equal(t, []uuid.UUID{pid}, all[1].CitedProductIDs)
	require.Equal(t, 12, all[1].TokensIn)

	recent, err := mr.ListRecent(ctx, c.ID, 2)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	require.Equal(t, "a1", recent[0].Content) // oldest of the last 2
	require.Equal(t, "q2", recent[1].Content)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./internal/stylist/repo/ -run TestMessagePG -v`
Expected: FAIL — `NewMessagePG` undefined.

- [ ] **Step 3: Implement the repo**

Create `internal/stylist/repo/message_pg.go`:

```go
package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

type MessagePG struct{ db DBTX }

func NewMessagePG(db DBTX) *MessagePG { return &MessagePG{db: db} }

func (r *MessagePG) Insert(ctx context.Context, m *domain.Message) error {
	cited := m.CitedProductIDs
	if cited == nil {
		cited = []uuid.UUID{}
	}
	return r.db.QueryRow(ctx,
		`INSERT INTO ai_messages (conversation_id, role, content, cited_product_ids, tokens_in, tokens_out, model)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		m.ConversationID, string(m.Role), m.Content, cited, m.TokensIn, m.TokensOut, m.Model,
	).Scan(&m.ID, &m.CreatedAt)
}

func scanMessages(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]*domain.Message, error) {
	defer rows.Close()
	var out []*domain.Message
	for rows.Next() {
		m := &domain.Message{}
		var role string
		if err := rows.Scan(&m.ID, &m.ConversationID, &role, &m.Content, &m.CitedProductIDs, &m.TokensIn, &m.TokensOut, &m.Model, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Role = domain.Role(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MessagePG) ListByConversation(ctx context.Context, conversationID uuid.UUID) ([]*domain.Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, conversation_id, role, content, cited_product_ids, tokens_in, tokens_out, model, created_at
		   FROM ai_messages
		  WHERE conversation_id = $1
		  ORDER BY created_at ASC, id ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (r *MessagePG) ListRecent(ctx context.Context, conversationID uuid.UUID, limit int) ([]*domain.Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, conversation_id, role, content, cited_product_ids, tokens_in, tokens_out, model, created_at
		   FROM (
		     SELECT id, conversation_id, role, content, cited_product_ids, tokens_in, tokens_out, model, created_at
		       FROM ai_messages
		      WHERE conversation_id = $1
		      ORDER BY created_at DESC, id DESC
		      LIMIT $2
		   ) t
		  ORDER BY created_at ASC, id ASC`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}
```

> Note: pgx scans a `UUID[]` column straight into `[]uuid.UUID` via the google/uuid pgx integration already used elsewhere in this repo (e.g. order item arrays). If a scan type error surfaces, scan into `[]uuid.UUID` is correct for pgx/v5 with the stdlib uuid codec; do not switch to `pq.Array`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags integration ./internal/stylist/repo/ -run TestMessagePG -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stylist/repo/message_pg.go internal/stylist/repo/message_pg_test.go
git commit -m "feat(stylist): message Postgres repo"
```

---

## Phase 4 — Service

### Task 15: Quota gate (interface + Redis impl)

The chat service depends on a `QuotaGate` interface so it can be unit-tested with a fake. The Redis implementation is thin (INCR + EXPIRE), mirroring the existing redis-backed stores (`internal/auth/repo/*_redis.go`), which are not unit-tested in isolation.

**Files:**
- Create: `internal/stylist/service/quota.go`

- [ ] **Step 1: Write the gate + Redis impl**

Create `internal/stylist/service/quota.go`:

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// QuotaGate tracks per-user daily message usage.
type QuotaGate interface {
	// Count returns today's current usage without mutating it.
	Count(ctx context.Context, userID uuid.UUID) (int, error)
	// Incr increments today's usage and returns the new value, setting a 24h TTL
	// on first use of the day.
	Incr(ctx context.Context, userID uuid.UUID) (int, error)
}

// RedisQuotaGate keys usage by user + UTC date: ai:quota:{userID}:{yyyymmdd}.
type RedisQuotaGate struct{ rdb *redis.Client }

func NewRedisQuotaGate(rdb *redis.Client) *RedisQuotaGate { return &RedisQuotaGate{rdb: rdb} }

func quotaKey(userID uuid.UUID) string {
	return fmt.Sprintf("ai:quota:%s:%s", userID.String(), time.Now().UTC().Format("20060102"))
}

func (g *RedisQuotaGate) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := g.rdb.Get(ctx, quotaKey(userID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (g *RedisQuotaGate) Incr(ctx context.Context, userID uuid.UUID) (int, error) {
	key := quotaKey(userID)
	n, err := g.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		// First message today: expire the counter ~25h later so it self-cleans.
		_ = g.rdb.Expire(ctx, key, 25*time.Hour).Err()
	}
	return int(n), nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/stylist/service/`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/stylist/service/quota.go
git commit -m "feat(stylist): quota gate interface + Redis impl"
```

---

### Task 16: Prompts + intent + product retriever

**Files:**
- Create: `internal/stylist/service/prompt.go`
- Create: `internal/stylist/service/retriever.go`
- Test: `internal/stylist/service/retriever_test.go`

Design note: Pass-1 intent is consumed as a full-text `q` plus an optional price range — NOT as taxonomy slug filters. The model cannot reliably emit our exact category/style slugs, and the catalog's full-text search over product name/description is the robust grounding path. The intent prompt therefore asks the model to translate the user's request (incl. occasion, e.g. "đám cưới") into concrete garment-type keywords.

- [ ] **Step 1: Write the failing retriever test**

Create `internal/stylist/service/retriever_test.go`:

```go
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/service"
)

type fakeCatalog struct {
	gotQuery *productdomain.ListProductsQuery
	items    []*productdomain.CatalogItem
}

func (f *fakeCatalog) List(_ context.Context, q *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error) {
	f.gotQuery = q
	return f.items, len(f.items), nil, nil
}

func newCatalogItem(id, slug, name, brandSlug, brandName string, minPrice float64, img string) *productdomain.CatalogItem {
	ci := &productdomain.CatalogItem{BrandSlug: brandSlug, BrandName: brandName, MinPrice: minPrice}
	ci.Product.Slug = slug
	ci.Product.Name = name
	if img != "" {
		ci.PrimaryImage = &img
	}
	// ID lives on the embedded Product; parse a fixed UUID string for determinism.
	_ = id
	return ci
}

func TestRetriever_MapsIntentToQueryAndCards(t *testing.T) {
	fc := &fakeCatalog{items: []*productdomain.CatalogItem{
		newCatalogItem("", "midi-dress", "Midi Dress", "rep", "REP", 650000, "https://img/1.jpg"),
	}}
	r := service.NewProductRetriever(fc, 6)

	pmax := 800000.0
	cards, err := r.Retrieve(context.Background(), service.Intent{Keywords: "đầm dự tiệc", PriceMax: &pmax})
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "midi-dress", cards[0].Slug)
	require.Equal(t, "rep", cards[0].BrandSlug)
	require.EqualValues(t, 650000, cards[0].PriceVND)

	require.Equal(t, "đầm dự tiệc", fc.gotQuery.Q)
	require.NotNil(t, fc.gotQuery.PriceMax)
	require.Equal(t, 6, fc.gotQuery.Limit)
	require.Equal(t, "relevance", fc.gotQuery.Sort)
}

func TestRetriever_EmptyKeywordsReturnsNoCards(t *testing.T) {
	fc := &fakeCatalog{}
	r := service.NewProductRetriever(fc, 6)
	cards, err := r.Retrieve(context.Background(), service.Intent{})
	require.NoError(t, err)
	require.Empty(t, cards)
	require.Nil(t, fc.gotQuery) // catalog not queried when there is nothing to search
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stylist/service/ -run TestRetriever -v`
Expected: FAIL — `NewProductRetriever`/`Intent` undefined.

- [ ] **Step 3: Implement the retriever**

Create `internal/stylist/service/retriever.go`:

```go
package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

// Intent is the structured output of Pass 1. Consumed as a full-text query + an
// optional price range (not taxonomy slugs — see retriever design note).
type Intent struct {
	Keywords string   `json:"keywords"`
	PriceMin *float64 `json:"price_min"`
	PriceMax *float64 `json:"price_max"`
}

// CatalogLister is satisfied by *productservice.CatalogService.
type CatalogLister interface {
	List(ctx context.Context, q *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error)
}

type ProductRetriever struct {
	catalog CatalogLister
	k       int
}

func NewProductRetriever(c CatalogLister, k int) *ProductRetriever {
	if k <= 0 {
		k = 6
	}
	return &ProductRetriever{catalog: c, k: k}
}

func (r *ProductRetriever) Retrieve(ctx context.Context, intent Intent) ([]domain.ProductCard, error) {
	q := strings.TrimSpace(intent.Keywords)
	if q == "" {
		return nil, nil
	}
	items, _, _, err := r.catalog.List(ctx, &productdomain.ListProductsQuery{
		Q:        q,
		PriceMin: intent.PriceMin,
		PriceMax: intent.PriceMax,
		Sort:     "relevance",
		Page:     1,
		Limit:    r.k,
	})
	if err != nil {
		return nil, err
	}
	cards := make([]domain.ProductCard, 0, len(items))
	for _, it := range items {
		cards = append(cards, domain.ProductCard{
			ID:              it.Product.ID,
			Slug:            it.Slug,
			Name:            it.Name,
			BrandSlug:       it.BrandSlug,
			BrandName:       it.BrandName,
			PrimaryImageURL: it.PrimaryImage,
			PriceVND:        int64(it.MinPrice),
		})
	}
	_ = uuid.Nil // ID is set on the embedded Product by the catalog repo
	return cards, nil
}
```

- [ ] **Step 4: Run retriever test to verify it passes**

Run: `go test ./internal/stylist/service/ -run TestRetriever -v`
Expected: PASS.

- [ ] **Step 5: Implement prompts**

Create `internal/stylist/service/prompt.go`:

```go
package service

import (
	"encoding/json"
	"strings"

	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
)

// intentSystemPrompt instructs Pass 1 to emit strict JSON search params.
const intentSystemPrompt = `You extract product-search parameters from a fashion shopper's message for an e-commerce catalog search.
Return ONLY a JSON object with this exact shape:
{"keywords": string, "price_min": number|null, "price_max": number|null}
Rules:
- "keywords": concrete Vietnamese garment-type search terms for the request. Translate occasions into garment types (e.g. "đám cưới" -> "đầm dự tiệc, áo sơ mi"). Empty string if the message is not about finding clothing.
- "price_min"/"price_max": VND budget if the user states one, else null.
Do not include any text outside the JSON object.`

// baseSystemPrompt is the Pass 2 persona + guardrails.
const baseSystemPrompt = `Bạn là stylist AI của WearWhere — sàn thời trang cho local brand Việt Nam.
Trả lời ngắn gọn, thân thiện, bằng tiếng Việt. Tư vấn phối đồ và gợi ý dựa trên các sản phẩm được cung cấp.
Chỉ nhắc tới các sản phẩm trong danh sách "Sản phẩm gợi ý" bên dưới; KHÔNG bịa tên sản phẩm hay đường link.
Nếu câu hỏi không liên quan tới thời trang/mua sắm, hãy từ chối lịch sự và mời người dùng hỏi về phong cách hoặc sản phẩm.`

// BuildSystemPrompt assembles the Pass 2 system instruction. profile is a
// forward-compat seam for B1 (always nil today); when set, it appends a
// personalization section.
func BuildSystemPrompt(profile *domain.StyleProfile, cards []domain.ProductCard) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt)

	if profile != nil {
		b.WriteString("\n\nHồ sơ phong cách của người dùng:\n")
		if len(profile.Styles) > 0 {
			b.WriteString("- Phong cách: " + strings.Join(profile.Styles, ", ") + "\n")
		}
		if len(profile.Sizes) > 0 {
			b.WriteString("- Size: " + strings.Join(profile.Sizes, ", ") + "\n")
		}
	}

	b.WriteString("\n\nSản phẩm gợi ý (JSON):\n")
	b.WriteString(productsJSON(cards))
	return b.String()
}

func productsJSON(cards []domain.ProductCard) string {
	type compact struct {
		Name     string `json:"name"`
		Brand    string `json:"brand"`
		PriceVND int64  `json:"price_vnd"`
		Slug     string `json:"slug"`
	}
	out := make([]compact, 0, len(cards))
	for _, c := range cards {
		out = append(out, compact{Name: c.Name, Brand: c.BrandName, PriceVND: c.PriceVND, Slug: c.Slug})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}
```

- [ ] **Step 6: Verify package compiles**

Run: `go test ./internal/stylist/service/ -run TestRetriever -v`
Expected: PASS (package still builds with the new prompt.go).

- [ ] **Step 7: Commit**

```bash
git add internal/stylist/service/prompt.go internal/stylist/service/retriever.go internal/stylist/service/retriever_test.go
git commit -m "feat(stylist): intent prompt, system prompt, catalog retriever"
```

---

### Task 17: Chat service orchestrator

**Files:**
- Create: `internal/stylist/service/chat_service.go`
- Test: `internal/stylist/service/chat_service_test.go`

- [ ] **Step 1: Write the failing test (fakes for repos, quota, llm)**

Create `internal/stylist/service/chat_service_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	stylistrepo "github.com/wearwhere/wearwhere_be/internal/stylist/repo"
	"github.com/wearwhere/wearwhere_be/internal/stylist/service"
)

// ── fakes ──

type fakeConvoRepo struct {
	convos map[uuid.UUID]*domain.Conversation
}

func newFakeConvoRepo() *fakeConvoRepo { return &fakeConvoRepo{convos: map[uuid.UUID]*domain.Conversation{}} }

func (f *fakeConvoRepo) Create(_ context.Context, c *domain.Conversation) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	f.convos[c.ID] = c
	return nil
}
func (f *fakeConvoRepo) FindByID(_ context.Context, id, userID uuid.UUID) (*domain.Conversation, error) {
	c, ok := f.convos[id]
	if !ok || c.UserID != userID || c.ArchivedAt != nil {
		return nil, stylistrepo.ErrNotFound
	}
	return c, nil
}
func (f *fakeConvoRepo) List(_ context.Context, userID uuid.UUID, _, _ int) ([]*domain.Conversation, int, error) {
	var out []*domain.Conversation
	for _, c := range f.convos {
		if c.UserID == userID && c.ArchivedAt == nil {
			out = append(out, c)
		}
	}
	return out, len(out), nil
}
func (f *fakeConvoRepo) Touch(_ context.Context, id uuid.UUID, ts time.Time, title string) error {
	if c, ok := f.convos[id]; ok {
		c.LastMessageAt = &ts
		c.Title = title
	}
	return nil
}
func (f *fakeConvoRepo) Rename(_ context.Context, id, userID uuid.UUID, title string) (*domain.Conversation, error) {
	c, err := f.FindByID(context.Background(), id, userID)
	if err != nil {
		return nil, err
	}
	c.Title = title
	return c, nil
}
func (f *fakeConvoRepo) Archive(_ context.Context, id, userID uuid.UUID) error {
	c, err := f.FindByID(context.Background(), id, userID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	c.ArchivedAt = &now
	return nil
}

type fakeMsgRepo struct{ byConvo map[uuid.UUID][]*domain.Message }

func newFakeMsgRepo() *fakeMsgRepo { return &fakeMsgRepo{byConvo: map[uuid.UUID][]*domain.Message{}} }

func (f *fakeMsgRepo) Insert(_ context.Context, m *domain.Message) error {
	m.ID = uuid.New()
	m.CreatedAt = time.Now().UTC()
	f.byConvo[m.ConversationID] = append(f.byConvo[m.ConversationID], m)
	return nil
}
func (f *fakeMsgRepo) ListByConversation(_ context.Context, id uuid.UUID) ([]*domain.Message, error) {
	return f.byConvo[id], nil
}
func (f *fakeMsgRepo) ListRecent(_ context.Context, id uuid.UUID, limit int) ([]*domain.Message, error) {
	all := f.byConvo[id]
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

type fakeQuota struct {
	count int
	err   error
}

func (f *fakeQuota) Count(context.Context, uuid.UUID) (int, error) { return f.count, f.err }
func (f *fakeQuota) Incr(context.Context, uuid.UUID) (int, error) {
	f.count++
	return f.count, f.err
}

func newSvc(t *testing.T, l llm.Client, q service.QuotaGate, cat service.CatalogLister) (*service.ChatService, *fakeConvoRepo, *fakeMsgRepo) {
	t.Helper()
	cr := newFakeConvoRepo()
	mr := newFakeMsgRepo()
	svc := service.NewChatService(cr, mr, l, service.NewProductRetriever(cat, 6), q, service.Config{
		DailyMessageLimit:  30,
		MaxContextMessages: 10,
	})
	return svc, cr, mr
}

func TestSendMessage_HappyPath(t *testing.T) {
	mock := llm.NewMockClient()
	mock.AnswerText = "Gợi ý phối đồ"
	fc := &fakeCatalog{} // empty catalog -> empty cards, still a valid response
	svc, cr, mr := newSvc(t, mock, &fakeQuota{}, fc)

	user := uuid.New()
	c := &domain.Conversation{UserID: user}
	require.NoError(t, cr.Create(context.Background(), c))

	res, err := svc.SendMessage(context.Background(), user, c.ID, "mặc gì đi làm?")
	require.NoError(t, err)
	require.Equal(t, domain.RoleUser, res.UserMessage.Role)
	require.Equal(t, "mặc gì đi làm?", res.UserMessage.Content)
	require.Equal(t, domain.RoleAssistant, res.AssistantMessage.Role)
	require.Equal(t, "Gợi ý phối đồ", res.AssistantMessage.Content)
	require.Equal(t, 1, res.Quota.Used)
	require.Equal(t, 29, res.Quota.Remaining)

	// both messages persisted
	all, _ := mr.ListByConversation(context.Background(), c.ID)
	require.Len(t, all, 2)
	// title derived from first user message
	got, _ := cr.FindByID(context.Background(), c.ID, user)
	require.Equal(t, "mặc gì đi làm?", got.Title)
	require.NotNil(t, got.LastMessageAt)
}

func TestSendMessage_QuotaExceeded(t *testing.T) {
	svc, cr, mr := newSvc(t, llm.NewMockClient(), &fakeQuota{count: 30}, &fakeCatalog{})
	user := uuid.New()
	c := &domain.Conversation{UserID: user}
	_ = cr.Create(context.Background(), c)

	_, err := svc.SendMessage(context.Background(), user, c.ID, "hi")
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit")
	// nothing persisted
	all, _ := mr.ListByConversation(context.Background(), c.ID)
	require.Empty(t, all)
}

func TestSendMessage_NotOwnerIsNotFound(t *testing.T) {
	svc, cr, _ := newSvc(t, llm.NewMockClient(), &fakeQuota{}, &fakeCatalog{})
	owner := uuid.New()
	c := &domain.Conversation{UserID: owner}
	_ = cr.Create(context.Background(), c)

	_, err := svc.SendMessage(context.Background(), uuid.New(), c.ID, "hi")
	require.ErrorIs(t, err, domain.ErrConversationNotFound)
}

func TestSendMessage_ProviderFailureOnComposeIsUnavailable(t *testing.T) {
	// Mock that succeeds on JSON (intent) but fails on the text (compose) call.
	failOnText := &textFailLLM{}
	svc, cr, mr := newSvc(t, failOnText, &fakeQuota{}, &fakeCatalog{})
	user := uuid.New()
	c := &domain.Conversation{UserID: user}
	_ = cr.Create(context.Background(), c)

	_, err := svc.SendMessage(context.Background(), user, c.ID, "hi")
	require.ErrorIs(t, err, domain.ErrProviderUnavailable)
	// the user message remains persisted; assistant message does not
	all, _ := mr.ListByConversation(context.Background(), c.ID)
	require.Len(t, all, 1)
	require.Equal(t, domain.RoleUser, all[0].Role)
}

type textFailLLM struct{}

func (textFailLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if req.JSON {
		return &llm.GenerateResponse{Text: `{"keywords":"x"}`, Model: "mock"}, nil
	}
	return nil, errors.New("boom")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stylist/service/ -run TestSendMessage -v`
Expected: FAIL — `NewChatService`/`Config`/`ChatService` undefined.

- [ ] **Step 3: Implement the chat service**

Create `internal/stylist/service/chat_service.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/repo"
)

type Config struct {
	DailyMessageLimit  int
	MaxContextMessages int
}

type ChatService struct {
	convos    repo.ConversationRepo
	messages  repo.MessageRepo
	llm       llm.Client
	retriever *ProductRetriever
	quota     QuotaGate
	cfg       Config
}

func NewChatService(
	convos repo.ConversationRepo,
	messages repo.MessageRepo,
	client llm.Client,
	retriever *ProductRetriever,
	quota QuotaGate,
	cfg Config,
) *ChatService {
	return &ChatService{convos: convos, messages: messages, llm: client, retriever: retriever, quota: quota, cfg: cfg}
}

// SendResult is the outcome of a SendMessage call.
type SendResult struct {
	UserMessage      *domain.Message
	AssistantMessage *domain.Message
	Cards            []domain.ProductCard
	Quota            domain.QuotaResponse
}

func (s *ChatService) CreateConversation(ctx context.Context, userID uuid.UUID, firstMessage string) (*domain.Conversation, *SendResult, error) {
	c := &domain.Conversation{UserID: userID}
	if err := s.convos.Create(ctx, c); err != nil {
		return nil, nil, err
	}
	firstMessage = strings.TrimSpace(firstMessage)
	if firstMessage == "" {
		return c, nil, nil
	}
	res, err := s.SendMessage(ctx, userID, c.ID, firstMessage)
	if err != nil {
		// Conversation was created; surface the send error so the caller can decide.
		return c, nil, err
	}
	// reload to reflect last_message_at/title set during SendMessage
	c, _ = s.convos.FindByID(ctx, c.ID, userID)
	return c, res, nil
}

func (s *ChatService) ListConversations(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Conversation, int, error) {
	return s.convos.List(ctx, userID, limit, offset)
}

func (s *ChatService) GetConversation(ctx context.Context, userID, id uuid.UUID) (*domain.Conversation, []*domain.Message, error) {
	c, err := s.convos.FindByID(ctx, id, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil, domain.ErrConversationNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	msgs, err := s.messages.ListByConversation(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return c, msgs, nil
}

func (s *ChatService) Rename(ctx context.Context, userID, id uuid.UUID, title string) (*domain.Conversation, error) {
	c, err := s.convos.Rename(ctx, id, userID, title)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrConversationNotFound
	}
	return c, err
}

func (s *ChatService) Archive(ctx context.Context, userID, id uuid.UUID) error {
	err := s.convos.Archive(ctx, id, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return domain.ErrConversationNotFound
	}
	return err
}

func (s *ChatService) SendMessage(ctx context.Context, userID, convoID uuid.UUID, content string) (*SendResult, error) {
	convo, err := s.convos.FindByID(ctx, convoID, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}

	used, err := s.quota.Count(ctx, userID)
	if err != nil {
		return nil, err
	}
	if used >= s.cfg.DailyMessageLimit {
		return nil, domain.QuotaExceeded(s.cfg.DailyMessageLimit, used)
	}

	userMsg := &domain.Message{ConversationID: convoID, Role: domain.RoleUser, Content: content}
	if err := s.messages.Insert(ctx, userMsg); err != nil {
		return nil, err
	}
	newUsed, err := s.quota.Incr(ctx, userID)
	if err != nil {
		return nil, err
	}

	history, err := s.messages.ListRecent(ctx, convoID, s.cfg.MaxContextMessages)
	if err != nil {
		return nil, err
	}

	intent := s.extractIntent(ctx, history)
	cards, err := s.retriever.Retrieve(ctx, intent)
	if err != nil {
		return nil, err
	}

	answer, usage, model, err := s.compose(ctx, history, cards)
	if err != nil {
		return nil, domain.ErrProviderUnavailable
	}

	assistantMsg := &domain.Message{
		ConversationID:  convoID,
		Role:            domain.RoleAssistant,
		Content:         answer,
		CitedProductIDs: cardIDs(cards),
		TokensIn:        usage.InputTokens,
		TokensOut:       usage.OutputTokens,
		Model:           &model,
	}
	if err := s.messages.Insert(ctx, assistantMsg); err != nil {
		return nil, err
	}

	title := convo.Title
	if strings.TrimSpace(title) == "" {
		title = deriveTitle(content)
	}
	_ = s.convos.Touch(ctx, convoID, time.Now().UTC(), title)

	return &SendResult{
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
		Cards:            cards,
		Quota:            domain.QuotaResponse{Used: newUsed, Limit: s.cfg.DailyMessageLimit, Remaining: max(0, s.cfg.DailyMessageLimit-newUsed)},
	}, nil
}

// extractIntent runs Pass 1. Any failure falls back to using the latest user
// message verbatim as the search keywords — intent extraction never blocks chat.
func (s *ChatService) extractIntent(ctx context.Context, history []*domain.Message) Intent {
	latest := latestUserContent(history)
	fallback := Intent{Keywords: latest}

	resp, err := s.llm.Generate(ctx, llm.GenerateRequest{
		System:      intentSystemPrompt,
		Messages:    toLLMMessages(history),
		Temperature: 0,
		JSON:        true,
	})
	if err != nil {
		return fallback
	}
	var intent Intent
	if err := json.Unmarshal([]byte(resp.Text), &intent); err != nil {
		return fallback
	}
	if strings.TrimSpace(intent.Keywords) == "" {
		intent.Keywords = latest
	}
	return intent
}

// compose runs Pass 2 and returns text + summed-not-needed single-call usage.
func (s *ChatService) compose(ctx context.Context, history []*domain.Message, cards []domain.ProductCard) (string, llm.Usage, string, error) {
	resp, err := s.llm.Generate(ctx, llm.GenerateRequest{
		System:      BuildSystemPrompt(nil, cards),
		Messages:    toLLMMessages(history),
		Temperature: 0.7,
	})
	if err != nil {
		return "", llm.Usage{}, "", err
	}
	return resp.Text, resp.Usage, resp.Model, nil
}

// ── helpers ──

func toLLMMessages(history []*domain.Message) []llm.Message {
	out := make([]llm.Message, 0, len(history))
	for _, m := range history {
		role := llm.RoleUser
		if m.Role == domain.RoleAssistant {
			role = llm.RoleModel
		}
		out = append(out, llm.Message{Role: role, Content: m.Content})
	}
	return out
}

func latestUserContent(history []*domain.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == domain.RoleUser {
			return history[i].Content
		}
	}
	return ""
}

func cardIDs(cards []domain.ProductCard) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	return ids
}

func deriveTitle(content string) string {
	t := strings.TrimSpace(content)
	r := []rune(t)
	if len(r) > 60 {
		return strings.TrimSpace(string(r[:60]))
	}
	return t
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

> Note: `max` is defined locally for clarity/compatibility; if the module's Go toolchain treats the builtin `max` as available and the compiler flags a redeclaration, delete the local `max` func and use the builtin. Verify with the build in Step 4.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/stylist/service/ -v`
Expected: PASS (retriever + chat service). If the build fails on `max` redeclaration, remove the local `max` per the note and re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/stylist/service/chat_service.go internal/stylist/service/chat_service_test.go
git commit -m "feat(stylist): chat service orchestrating the RAG-lite pipeline"
```

---

## Phase 5 — Handler

### Task 18: HTTP handler + routes

**Files:**
- Create: `internal/stylist/handler/handler.go`
- Create: `internal/stylist/handler/routes.go`
- Test: `internal/stylist/handler/handler_test.go`

- [ ] **Step 1: Write the failing handler test**

Create `internal/stylist/handler/handler_test.go`:

```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	stylisthandler "github.com/wearwhere/wearwhere_be/internal/stylist/handler"
	stylistservice "github.com/wearwhere/wearwhere_be/internal/stylist/service"
)

// inMemoryDeps wires the real service against in-memory fakes for an HTTP-level
// test without a database. Reuse the service-package fakes by constructing the
// service the same way; here we use a minimal in-test catalog + quota.
//
// To avoid duplicating the service fakes, this handler test exercises only the
// create + send happy path and the validation error path, which need just the
// public service surface.

func setupRouter(t *testing.T, userID uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cr := stylistservice.NewInMemoryConversationRepoForTest()
	mr := stylistservice.NewInMemoryMessageRepoForTest()
	cat := stylistservice.NewEmptyCatalogForTest()
	svc := stylistservice.NewChatService(cr, mr, llm.NewMockClient(),
		stylistservice.NewProductRetriever(cat, 6),
		stylistservice.NewNoopQuotaForTest(),
		stylistservice.Config{DailyMessageLimit: 30, MaxContextMessages: 10},
	)

	h := stylisthandler.New(svc)
	r := gin.New()
	grp := r.Group("/me", func(c *gin.Context) {
		authmw.SetUserIDForTest(c, userID)
		c.Next()
	})
	stylisthandler.Mount(grp, h)
	return r
}

func TestCreateAndSend(t *testing.T) {
	user := uuid.New()
	r := setupRouter(t, user)

	// create with first message
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/me/stylist/conversations",
		strings.NewReader(`{"first_message":"mặc gì đi biển?"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var detail map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	msgs := detail["messages"].([]any)
	require.Len(t, msgs, 2) // user + assistant
}

func TestSend_EmptyContentIsBadRequest(t *testing.T) {
	user := uuid.New()
	r := setupRouter(t, user)

	// first create an empty conversation
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/me/stylist/conversations",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var detail map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	id := detail["id"].(string)

	// send empty content
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/me/stylist/conversations/"+id+"/messages",
		strings.NewReader(`{"content":""}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusBadRequest, w2.Code)
}
```

This test references small test-only constructors. Add them to the service package in Step 2 before implementing the handler.

- [ ] **Step 2: Add test-only in-memory constructors to the service package**

Create `internal/stylist/service/testhelpers.go`:

```go
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/repo"
)

// These constructors back HTTP-layer tests in other packages that need a working
// ChatService without a database. They are intentionally simple and exported.

type inMemConvoRepo struct{ m map[uuid.UUID]*domain.Conversation }

func NewInMemoryConversationRepoForTest() repo.ConversationRepo {
	return &inMemConvoRepo{m: map[uuid.UUID]*domain.Conversation{}}
}
func (r *inMemConvoRepo) Create(_ context.Context, c *domain.Conversation) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	r.m[c.ID] = c
	return nil
}
func (r *inMemConvoRepo) FindByID(_ context.Context, id, userID uuid.UUID) (*domain.Conversation, error) {
	c, ok := r.m[id]
	if !ok || c.UserID != userID || c.ArchivedAt != nil {
		return nil, repo.ErrNotFound
	}
	return c, nil
}
func (r *inMemConvoRepo) List(_ context.Context, userID uuid.UUID, _, _ int) ([]*domain.Conversation, int, error) {
	var out []*domain.Conversation
	for _, c := range r.m {
		if c.UserID == userID && c.ArchivedAt == nil {
			out = append(out, c)
		}
	}
	return out, len(out), nil
}
func (r *inMemConvoRepo) Touch(_ context.Context, id uuid.UUID, ts time.Time, title string) error {
	if c, ok := r.m[id]; ok {
		c.LastMessageAt = &ts
		c.Title = title
	}
	return nil
}
func (r *inMemConvoRepo) Rename(_ context.Context, id, userID uuid.UUID, title string) (*domain.Conversation, error) {
	c, err := r.FindByID(context.Background(), id, userID)
	if err != nil {
		return nil, err
	}
	c.Title = title
	return c, nil
}
func (r *inMemConvoRepo) Archive(_ context.Context, id, userID uuid.UUID) error {
	c, err := r.FindByID(context.Background(), id, userID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	c.ArchivedAt = &now
	return nil
}

type inMemMsgRepo struct{ m map[uuid.UUID][]*domain.Message }

func NewInMemoryMessageRepoForTest() repo.MessageRepo {
	return &inMemMsgRepo{m: map[uuid.UUID][]*domain.Message{}}
}
func (r *inMemMsgRepo) Insert(_ context.Context, m *domain.Message) error {
	m.ID = uuid.New()
	m.CreatedAt = time.Now().UTC()
	r.m[m.ConversationID] = append(r.m[m.ConversationID], m)
	return nil
}
func (r *inMemMsgRepo) ListByConversation(_ context.Context, id uuid.UUID) ([]*domain.Message, error) {
	return r.m[id], nil
}
func (r *inMemMsgRepo) ListRecent(_ context.Context, id uuid.UUID, limit int) ([]*domain.Message, error) {
	all := r.m[id]
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

type emptyCatalog struct{}

func NewEmptyCatalogForTest() CatalogLister { return emptyCatalog{} }
func (emptyCatalog) List(context.Context, *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error) {
	return nil, 0, nil, nil
}

type noopQuota struct{ n int }

func NewNoopQuotaForTest() QuotaGate { return &noopQuota{} }
func (q *noopQuota) Count(context.Context, uuid.UUID) (int, error) { return q.n, nil }
func (q *noopQuota) Incr(context.Context, uuid.UUID) (int, error)  { q.n++; return q.n, nil }
```

- [ ] **Step 3: Run handler test to verify it fails**

Run: `go test ./internal/stylist/handler/ -v`
Expected: FAIL — `stylisthandler.New`/`Mount` undefined.

- [ ] **Step 4: Implement the handler**

Create `internal/stylist/handler/handler.go`:

```go
// Package handler exposes HTTP endpoints for the AI stylist chatbot.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/stylist/domain"
	"github.com/wearwhere/wearwhere_be/internal/stylist/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct {
	svc *service.ChatService
}

func New(s *service.ChatService) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func paginate(c *gin.Context) (limit, offset, page int) {
	page = 1
	limit = 20
	if v := c.Query("page"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n <= 50 {
			limit = n
		}
	}
	offset = (page - 1) * limit
	return limit, offset, page
}

func (h *Handler) Create(c *gin.Context) {
	var req domain.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	convo, res, err := h.svc.CreateConversation(c.Request.Context(), h.userID(c), req.FirstMessage)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := domain.ConversationDetailResponse{ConversationResponse: domain.ToConversationResponse(convo)}
	if res != nil {
		out.Messages = []domain.MessageResponse{
			domain.ToMessageResponse(res.UserMessage, nil),
			domain.ToMessageResponse(res.AssistantMessage, res.Cards),
		}
	} else {
		out.Messages = []domain.MessageResponse{}
	}
	httpx.Created(c, out)
}

func (h *Handler) List(c *gin.Context) {
	limit, offset, page := paginate(c)
	items, total, err := h.svc.ListConversations(c.Request.Context(), h.userID(c), limit, offset)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	data := make([]domain.ConversationResponse, 0, len(items))
	for _, it := range items {
		data = append(data, domain.ToConversationResponse(it))
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}
	httpx.OK(c, gin.H{
		"data": data, "page": page, "page_size": limit, "total": total, "total_pages": totalPages,
	})
}

func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrConversationNotFound)
		return
	}
	convo, msgs, err := h.svc.GetConversation(c.Request.Context(), h.userID(c), id)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := domain.ConversationDetailResponse{ConversationResponse: domain.ToConversationResponse(convo)}
	out.Messages = make([]domain.MessageResponse, 0, len(msgs))
	for _, m := range msgs {
		// History detail does not re-hydrate product cards (cited_product_ids are
		// stored but cards are only attached on the live send response).
		out.Messages = append(out.Messages, domain.ToMessageResponse(m, nil))
	}
	httpx.OK(c, out)
}

func (h *Handler) SendMessage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrConversationNotFound)
		return
	}
	var req domain.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	res, err := h.svc.SendMessage(c.Request.Context(), h.userID(c), id, req.Content)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.SendMessageResponse{
		UserMessage:      domain.ToMessageResponse(res.UserMessage, nil),
		AssistantMessage: domain.ToMessageResponse(res.AssistantMessage, res.Cards),
		Quota:            res.Quota,
	})
}

func (h *Handler) Rename(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrConversationNotFound)
		return
	}
	var req domain.RenameConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	convo, err := h.svc.Rename(c.Request.Context(), h.userID(c), id, req.Title)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToConversationResponse(convo))
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrConversationNotFound)
		return
	}
	if err := h.svc.Archive(c.Request.Context(), h.userID(c), id); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
```

Create `internal/stylist/handler/parse.go`:

```go
package handler

import "strconv"

func parsePositiveInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}
```

Create `internal/stylist/handler/routes.go`:

```go
package handler

import "github.com/gin-gonic/gin"

// Mount registers stylist routes onto a group that already has RequireAuth +
// RequireRole(customer) applied (e.g., /me).
func Mount(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/stylist")
	g.POST("/conversations", h.Create)
	g.GET("/conversations", h.List)
	g.GET("/conversations/:id", h.Get)
	g.POST("/conversations/:id/messages", h.SendMessage)
	g.PATCH("/conversations/:id", h.Rename)
	g.DELETE("/conversations/:id", h.Delete)
}
```

- [ ] **Step 5: Run handler test to verify it passes**

Run: `go test ./internal/stylist/handler/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/stylist/handler/ internal/stylist/service/testhelpers.go
git commit -m "feat(stylist): HTTP handler + routes"
```

---

## Phase 6 — Wiring + E2E

### Task 19: Wire the module into `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add imports**

In the import block of `cmd/api/main.go`, add (keep grouping/alias style consistent):

```go
	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	stylisthandler "github.com/wearwhere/wearwhere_be/internal/stylist/handler"
	stylistrepo "github.com/wearwhere/wearwhere_be/internal/stylist/repo"
	stylistservice "github.com/wearwhere/wearwhere_be/internal/stylist/service"
```

- [ ] **Step 2: Build the LLM client (near the other client constructors, after the PayOS client block)**

```go
	// ── LLM client (AI stylist) ──
	llmClient, err := llm.NewFromConfig(llm.Config{
		Provider: cfg.AI.Provider,
		APIKey:   cfg.AI.GeminiAPIKey,
		Model:    cfg.AI.GeminiModel,
		Timeout:  cfg.AI.RequestTimeout,
	})
	if err != nil {
		log.Fatalf("llm client: %v", err)
	}
```

- [ ] **Step 3: Build the stylist service (after `catalogSvc` is constructed — it is the retriever's catalog dependency)**

Add near the other service constructions (after `cartSvc := ...`):

```go
	stylistConvoRepo := stylistrepo.NewConversationPG(pgPool)
	stylistMsgRepo := stylistrepo.NewMessagePG(pgPool)
	stylistSvc := stylistservice.NewChatService(
		stylistConvoRepo, stylistMsgRepo, llmClient,
		stylistservice.NewProductRetriever(catalogSvc, cfg.AI.RetrieveK),
		stylistservice.NewRedisQuotaGate(rdb),
		stylistservice.Config{
			DailyMessageLimit:  cfg.AI.DailyMessageLimit,
			MaxContextMessages: cfg.AI.MaxContextMessages,
		},
	)
	stylistHandler := stylisthandler.New(stylistSvc)
```

> `catalogSvc` is `*productservice.CatalogService`, whose `List(ctx, *ListProductsQuery) ([]*CatalogItem, int, []string, error)` satisfies `stylistservice.CatalogLister`.

- [ ] **Step 4: Mount routes on the existing `customerGroup`**

After `orderhandler.Mount(customerGroup, orderH)`:

```go
	stylisthandler.Mount(customerGroup, stylistHandler)
```

- [ ] **Step 5: Verify the binary builds**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(stylist): wire AI stylist module into the API"
```

---

### Task 20: End-to-end test (integration)

Exercises the full HTTP path with `AI_PROVIDER=mock`: create → send → list → get → rename → delete. Asserts plumbing, quota fields, and persistence. Product-match correctness is covered by the retriever unit test, so this asserts `products` is a (possibly empty) array rather than depending on full-text search.

**Files:**
- Create: `cmd/api/stylist_e2e_test.go`

- [ ] **Step 1: Inspect an existing E2E helper to reuse the harness**

Read `cmd/api/main_test.go` to find the existing test router builder + auth-token helper (e.g. a `setupTestServer`/`buildRouter` function and a helper that registers/logs in a customer and returns a bearer token). Reuse those exact helpers; do not duplicate router construction.

- [ ] **Step 2: Write the E2E test**

Create `cmd/api/stylist_e2e_test.go` (adapt the harness/helper names to whatever `main_test.go` actually exports — placeholders below are `newTestServer`, `registerCustomer`, and `doJSON`, which you will replace with the real helpers found in Step 1):

```go
//go:build integration

package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStylistChat_E2E(t *testing.T) {
	ts := newTestServer(t)          // existing harness: *httptest.Server-like with the full router + mock providers
	token := registerCustomer(t, ts) // existing helper: returns a customer bearer token

	// 1) create a conversation with a first message
	var createResp struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Messages []struct {
			Role     string          `json:"role"`
			Content  string          `json:"content"`
			Products []map[string]any `json:"products"`
		} `json:"messages"`
	}
	status := doJSON(t, ts, http.MethodPost, "/api/v1/me/stylist/conversations", token,
		map[string]any{"first_message": "mặc gì đi làm?"}, &createResp)
	require.Equal(t, http.StatusCreated, status)
	require.Len(t, createResp.Messages, 2)
	require.Equal(t, "user", createResp.Messages[0].Role)
	require.Equal(t, "assistant", createResp.Messages[1].Role)
	require.NotEmpty(t, createResp.Messages[1].Content)
	require.NotEmpty(t, createResp.ID)

	convoID := createResp.ID

	// 2) send another message
	var sendResp struct {
		AssistantMessage struct {
			Content string `json:"content"`
		} `json:"assistant_message"`
		Quota struct {
			Used      int `json:"used"`
			Limit     int `json:"limit"`
			Remaining int `json:"remaining"`
		} `json:"quota"`
	}
	status = doJSON(t, ts, http.MethodPost, "/api/v1/me/stylist/conversations/"+convoID+"/messages", token,
		map[string]any{"content": "còn đi tiệc thì sao?"}, &sendResp)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, sendResp.AssistantMessage.Content)
	require.Equal(t, 2, sendResp.Quota.Used) // first message + this one
	require.Equal(t, 30, sendResp.Quota.Limit)
	require.Equal(t, 28, sendResp.Quota.Remaining)

	// 3) list shows the conversation
	var listResp struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	status = doJSON(t, ts, http.MethodGet, "/api/v1/me/stylist/conversations", token, nil, &listResp)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 1, listResp.Total)

	// 4) get detail returns all messages (2 exchanges = 4 messages)
	var detail struct {
		Messages []map[string]any `json:"messages"`
	}
	status = doJSON(t, ts, http.MethodGet, "/api/v1/me/stylist/conversations/"+convoID, token, nil, &detail)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, detail.Messages, 4)

	// 5) rename
	var renamed struct {
		Title string `json:"title"`
	}
	status = doJSON(t, ts, http.MethodPatch, "/api/v1/me/stylist/conversations/"+convoID, token,
		map[string]any{"title": "Tư vấn công sở"}, &renamed)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Tư vấn công sở", renamed.Title)

	// 6) delete (archive) then it disappears from list
	status = doJSON(t, ts, http.MethodDelete, "/api/v1/me/stylist/conversations/"+convoID, token, nil, nil)
	require.Equal(t, http.StatusNoContent, status)

	status = doJSON(t, ts, http.MethodGet, "/api/v1/me/stylist/conversations", token, nil, &listResp)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 0, listResp.Total)
}
```

- [ ] **Step 3: Run the E2E test**

Run (PowerShell): set `TEST_DATABASE_URL` to a DB migrated through `000033`, then:
```
$env:AI_PROVIDER = "mock"
go test -tags integration ./cmd/api/ -run TestStylistChat_E2E -v
```
Expected: PASS. If helper names differ, fix the references (Step 1) until it compiles and passes.

- [ ] **Step 4: Commit**

```bash
git add cmd/api/stylist_e2e_test.go
git commit -m "test(stylist): end-to-end chat flow (mock provider)"
```

---

### Task 21: Full build + test sweep

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 2: Run all unit tests (untagged)**

Run: `go test ./...`
Expected: PASS, including `internal/shared/llm`, `internal/stylist/domain`, `internal/stylist/service`, `internal/stylist/handler`, `pkg/httpx`.

- [ ] **Step 3: Run integration tests**

Run (PowerShell, with `TEST_DATABASE_URL` set + DB migrated through `000033`):
```
$env:AI_PROVIDER = "mock"
go test -tags integration ./internal/stylist/... ./cmd/api/...
```
Expected: PASS.

- [ ] **Step 4: Vet**

Run: `go vet ./...`
Expected: no findings.

- [ ] **Step 5: Final commit (if vet/build required any fixups)**

```bash
git add -A
git commit -m "chore(stylist): build/test sweep fixups"
```

---

## Spec Coverage Map

| Spec section | Task(s) |
|--------------|---------|
| §1 Goals / §2 LLM provider seam | 3,4,5,6 |
| §2 grounding RAG-lite + retrieval (2-pass) | 16,17 |
| §2 product citation from DB | 16 (retriever), 17 (cardIDs), 11 (DTO) |
| §3 quota gate (Redis) | 15,17 |
| §3 token accounting per message | 14 (schema/insert), 17 (sum) |
| §3 provider failure → 502, user msg retained | 17 (test + impl) |
| §3 inappropriate query → in-band 200 decline | 16 (system prompt guardrail) |
| §4 data model: ai_conversations | 7,13 |
| §4 data model: ai_messages | 8,14 |
| §6 API surface (6 endpoints) | 18,19 |
| §6 response shapes (cards, quota) | 11,18 |
| §7 error codes (Format A + details) | 1,10,18 |
| §8 config (env) | 2 |
| §9 forward-compat (B1 seam) | 9 (StyleProfile), 16 (BuildSystemPrompt(nil,...)) |
| §10 testing (mock, unit, provider mapping, E2E) | 4,5,16,17,18,20,21 |

## Notes / Deviations from Spec (intentional)

- **Intent shape simplified** to `{keywords, price_min, price_max}` consumed as a full-text `q` + price range, rather than emitting taxonomy slug filters (`category`/`style_tags`). Rationale in Task 16: the model cannot reliably produce our exact catalog slugs; full-text search over name/description is the robust grounding path. The 2-pass structure (intent → retrieve → answer) is preserved.
- **Conversation detail (GET) does not re-hydrate product cards** for historical assistant messages — `cited_product_ids` are stored, but cards are attached only on the live send response. Re-hydration can be a later enhancement (file an issue if the FE needs it).
- **`NOT_OWNER` (403)** from the spec's error table is intentionally not emitted; owner mismatch returns `404 CONVERSATION_NOT_FOUND` to avoid resource enumeration (matches the address module's IDOR handling).
