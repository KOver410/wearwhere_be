# AI Stylist Chatbot (UC28) — Design

**Date:** 2026-06-05
**Branch (target):** dedicated `ai-stylist-chatbot` branch (off `main`)
**Status:** APPROVED FOR PLANNING
**Scope:** A conversational AI stylist for logged-in customers. Multi-turn chat persisted server-side, grounded in the real WearWhere product catalog via a retrieve-then-generate (RAG-lite) pipeline on the Gemini API. Per-user daily message quota + per-message token tracking. Single-JSON (non-streaming) responses.

This is **sub-feature A of 4** in the "AI & Personalization" area. Siblings (separate specs later): B1 Style Preferences (UC31), B2 AI Recommendations (UC29), B3 Smart Wardrobe (UC30). This spec is self-contained and does not depend on the others; it leaves a forward-compatible seam to consume the B1 style profile when it exists.

---

## 1. Goals

| Goal | Coverage |
|------|----------|
| Customer chats with an AI stylist for fashion advice | Full — conversation + message endpoints under `/api/v1/me/stylist` |
| Responses reference the real WearWhere catalog | Full — 2-pass RAG-lite: intent extraction → catalog query → grounded answer; product cards attached from our DB |
| Multi-turn context + chat history | Full — multiple conversations per user, last N messages sent as context |
| Cost / abuse control | Full — Redis per-user daily message cap + per-message token accounting in DB |
| Inappropriate queries handled gracefully | Full — in-band polite decline (200), not an error |
| Provider-agnostic, testable | Full — `llm.Client` port with `gemini` + `mock` adapters; deterministic tests |
| Latency within NFR (< 3s) | Targeted — Gemini **flash** model, bounded context, configurable timeout |

**Non-goals (deferred):**
- Streaming / token-by-token responses (chosen: single JSON).
- Function-calling / agentic tool loops (chosen: RAG-lite).
- Personalization from a style profile — seam left for B1, but B1 is not built here.
- Actions from chat (add-to-cart, place order). Chat returns product **links/cards** only.
- Recommendations feed (B2) and wardrobe outfit suggestions (B3).
- Chatbot for guest/brand/admin roles — customers only.

## 2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| LLM provider | **Gemini API** (`gemini-2.0-flash`) | Matches SRS Tools table; generous free tier fits "free tier limits"; flash keeps latency < 3s. (UC text said "Claude" — superseded.) |
| Provider seam | `internal/shared/llm` port + `gemini`/`mock` adapters, selected by `AI_PROVIDER` | Deterministic tests; reusable by future B2/B3 |
| Grounding | **RAG-lite, retrieve-then-generate** (not function-calling) | Simpler, deterministic, easy to test |
| Retrieval | **2-pass**: Gemini extracts structured search params → existing catalog query → Gemini composes answer | Full-text on raw advice queries ("mặc gì đi đám cưới") doesn't match product names; intent extraction maps to category/style_tags. Cost: 2 flash calls/message |
| Product citation | Cards built from **our DB**, not parsed from model text | Model may hallucinate names/links; we attach real products by id |
| Conversation persistence | DB, **multiple sessions per user** | Matches UC "open AI Stylist chat"; enables history + analytics |
| Response delivery | **Single JSON** (block then return) | Simplest for Go + Flutter/dio; product citations don't fit a token stream cleanly |
| Quota gate | **Redis** daily counter `ai:quota:{user_id}:{yyyymmdd}` (INCR + EXPIRE) | Cheap, matches existing Redis usage; no extra table |
| Token accounting | Stored per `ai_messages` row (`tokens_in`/`tokens_out`/`model`) | Audit/analytics without a usage table |
| Inappropriate query | In-band **200** decline message (system-prompt guardrail + Gemini safety) | Keeps the decline in chat history; matches UC alt-sequence |
| Error format | **Format A** (nested `{error:{code,message,details}}`) | Consistent with the other new modules (locations, fulfillment) |
| Route ownership | All conversation reads/writes scoped by `user_id` (IDOR-safe) | Same pattern as customer addresses/orders |

## 3. Architecture & Package Layout

```
db/migrations/
  000032_create_ai_conversations.{up,down}.sql      NEW
  000033_create_ai_messages.{up,down}.sql           NEW

internal/shared/llm/                                NEW (reusable across AI features)
  client.go         port: Client interface + GenerateRequest/GenerateResponse + Message/Usage types
  gemini.go         HTTP adapter -> Generative Language API (generateContent), maps usage metadata
  mock.go           canned responses for tests/dev (AI_PROVIDER=mock)
  factory.go        build Client from config (gemini|mock)
  gemini_test.go    request/response mapping, error mapping

internal/stylist/
  domain/
    conversation.go   Conversation, Message entities; Role enum
    dto.go            request/response DTOs + product card DTO
    errors.go         ErrConversationNotFound, ErrNotOwner, ErrQuotaExceeded, ErrProviderUnavailable
  repo/
    repo.go           interfaces (ConversationRepo, MessageRepo)
    conversation_pg.go
    message_pg.go
    *_test.go
  service/
    chat_service.go        orchestrates pipeline (quota -> context -> intent -> retrieve -> answer -> persist)
    chat_service_test.go   uses mock llm + fake/real repos + product retriever
    prompt.go              buildSystemPrompt(user, styleProfile?) + intent-extraction prompt
    retriever.go           ProductRetriever: params -> catalog query -> []ProductCard
    quota.go               Redis daily counter gate
  handler/
    routes.go         RegisterRoutes(rg, deps) under /api/v1/me/stylist (auth + role=customer)
    chat_handler.go   create/list/get/post-message/patch/delete

internal/config/         MODIFY  add AI/Gemini config block
cmd/api/                 MODIFY  wire llm.Client, stylist module; main_test.go E2E
```

**Dependency direction:** `stylist/service` depends on `llm.Client` (port), the existing product **catalog query/service** (for retrieval), and a Redis client (quota). It does not import handlers. The retriever wraps the existing catalog read path rather than duplicating SQL.

## 4. Data Model

### `ai_conversations`
| Column | Type | Notes |
|--------|------|-------|
| id | uuid PK | |
| user_id | uuid FK → users(id) | indexed; ON DELETE CASCADE |
| title | text | derived from first user message (truncated); editable |
| created_at | timestamptz | |
| updated_at | timestamptz | |
| last_message_at | timestamptz NULL | for list ordering |
| archived_at | timestamptz NULL | soft-archive (DELETE endpoint sets this) |

Index: `(user_id, last_message_at DESC)`.

### `ai_messages`
| Column | Type | Notes |
|--------|------|-------|
| id | uuid PK | |
| conversation_id | uuid FK → ai_conversations(id) | indexed; ON DELETE CASCADE |
| role | text | `'user'` \| `'assistant'` (CHECK) |
| content | text | |
| cited_product_ids | uuid[] | products attached to an assistant message (empty for user) |
| tokens_in | int | 0 for user rows; prompt tokens for assistant rows |
| tokens_out | int | 0 for user rows; completion tokens for assistant rows |
| model | text NULL | e.g. `gemini-2.0-flash` (assistant rows) |
| created_at | timestamptz | |

Index: `(conversation_id, created_at)`.

**Quota:** no table. Redis key `ai:quota:{user_id}:{yyyymmdd}` — `INCR` then `EXPIRE` 24h on first hit; gate compares against `AI_CHAT_DAILY_MESSAGE_LIMIT`. The counter is incremented **only after** a user message is accepted for processing (not on read endpoints).

## 5. RAG-lite Pipeline (POST a message)

On `POST /me/stylist/conversations/:id/messages` with `{content}`:

1. **Owner check** — load conversation by `(id, user_id)`; not found / not owned → `404 CONVERSATION_NOT_FOUND`.
2. **Quota gate** — Redis daily counter ≥ limit → `429 AI_QUOTA_EXCEEDED` (no message stored, no Gemini call).
3. **Persist user message** — insert `ai_messages` row (role=user). Increment quota counter.
4. **Load context** — last `AI_CHAT_MAX_CONTEXT_MESSAGES` (default 10) messages of this conversation, oldest→newest.
5. **Pass 1 — intent extraction** — Gemini call: given the new message + context, return strict JSON `{category?, style_tags?[], price_min?, price_max?, color?, keywords?}`. Parse defensively; on parse failure fall back to `{keywords: <raw message>}`.
6. **Retrieve** — `ProductRetriever` maps the params onto the existing catalog query (q/category/style_tags/price range/sort) and takes top `K` (default 6) active, in-stock products → `[]ProductCard`.
7. **Pass 2 — compose answer** — Gemini call: `system prompt + context + user message + compact JSON of the K products` → natural-language advice that references products by name. System prompt forbids inventing products/links and instructs polite decline for out-of-scope/inappropriate queries.
8. **Attach citations** — assistant `products[]` are built from our DB `ProductCard`s (subset the model actually referenced if detectable, else the retrieved set); `cited_product_ids` stored.
9. **Persist assistant message** — insert row with content, `cited_product_ids`, `tokens_in/out`, `model` (summed across both passes).
10. **Touch conversation** — `last_message_at = now()`, set `title` from first user message if empty.
11. **Respond** — `{user_message, assistant_message{...,products[]}, quota{used,limit,remaining}}`.

**Provider failure** (timeout / non-2xx / safety block on Pass 2): the user message stays stored; respond `502 AI_PROVIDER_UNAVAILABLE` so FE shows retry (UC alt-sequence 4a). Quota is **not** refunded (keeps logic simple; limit is generous).

**Inappropriate query:** handled in Pass 2 by the model returning a polite decline → normal `200` with an assistant message and empty `products[]`. Not an error.

## 6. API Surface

All under `/api/v1/me/stylist`, `auth` + `role=customer`. Errors are Format A.

| Method | Path | Body | Response |
|--------|------|------|----------|
| POST | `/conversations` | `{first_message?}` | `201` Conversation (+ first exchange if `first_message` given) |
| GET | `/conversations` | — | `200` paginated `{data,page,page_size,total,total_pages}` (non-archived, by `last_message_at DESC`) |
| GET | `/conversations/:id` | — | `200` Conversation + ordered `messages[]` |
| POST | `/conversations/:id/messages` | `{content}` (1..2000 chars) | `200` `{user_message, assistant_message, quota}` |
| PATCH | `/conversations/:id` | `{title}` (1..120) | `200` Conversation |
| DELETE | `/conversations/:id` | — | `204` (sets `archived_at`) |

**ProductCard DTO:** `{id, slug, name, brand:{slug,name}, primary_image_url, price_vnd}` (mirrors existing catalog card fields).

**POST message response example:**
```json
{
  "user_message": { "id":"uuid","role":"user","content":"Mặc gì đi đám cưới?","created_at":"..." },
  "assistant_message": {
    "id":"uuid","role":"assistant",
    "content":"Với tiệc cưới bạn có thể chọn...",
    "products":[
      {"id":"uuid","slug":"midi-dress","name":"Midi Dress","brand":{"slug":"rep-vn","name":"REP VN"},
       "primary_image_url":"https://...","price_vnd":650000}
    ],
    "created_at":"..."
  },
  "quota": { "used":4, "limit":30, "remaining":26 }
}
```

## 7. Error Codes (Format A)

| Code | HTTP | When |
|------|------|------|
| `CONVERSATION_NOT_FOUND` | 404 | id missing or not owned by caller |
| `NOT_OWNER` | 403 | (reserved; owner mismatch surfaces as 404 to avoid enumeration) |
| `AI_QUOTA_EXCEEDED` | 429 | daily message cap reached; `details:{limit,used}` |
| `AI_PROVIDER_UNAVAILABLE` | 502 | Gemini timeout / error / safety block on compose |
| `VALIDATION_FAILED` | 400 | empty/oversized `content` or `title` |

## 8. Config (env)

| Key | Default | Meaning |
|-----|---------|---------|
| `AI_PROVIDER` | `mock` | `gemini` \| `mock` |
| `GEMINI_API_KEY` | — | required when `AI_PROVIDER=gemini` |
| `GEMINI_MODEL` | `gemini-2.0-flash` | model id for both passes |
| `AI_CHAT_DAILY_MESSAGE_LIMIT` | `30` | per-user messages/day |
| `AI_CHAT_MAX_CONTEXT_MESSAGES` | `10` | prior messages sent as context |
| `AI_CHAT_RETRIEVE_K` | `6` | products retrieved per message |
| `AI_REQUEST_TIMEOUT` | `15s` | per Gemini HTTP call |

Default `AI_PROVIDER=mock` keeps dev/test/CI from needing a real key or hitting the network.

## 9. Forward-Compatibility (B1 seam)

`buildSystemPrompt(user, styleProfile *StyleProfile)` accepts an optional style profile. Today it is always `nil`. When B1 (UC31 Set Style Preferences) lands, the chat service fetches the profile and passes it in, so the stylist personalizes by preferred styles / budget / size with no API change.

## 10. Testing Strategy

- **`llm/mock`** returns canned, deterministic responses (incl. a fixed intent JSON and a fixed answer) so the whole pipeline is testable offline.
- **Unit:** quota gate (under/at/over limit, expiry set once), retriever param mapping, intent-JSON parse + fallback, prompt builder (with/without style profile), owner scoping (IDOR — other user's conversation → 404).
- **Provider mapping:** `gemini_test.go` covers request build + usage/error mapping (table-driven, no network).
- **E2E** (`cmd/api/main_test.go` style, `AI_PROVIDER=mock`): create conversation → post message → assert grounded products + quota fields → list shows it ordered → delete archives it.

## 11. Out-of-Scope / Follow-ups (file as issues)

- B1 Style Preferences, B2 Recommendations, B3 Smart Wardrobe (separate specs).
- Streaming responses; function-calling upgrade; chat-initiated cart actions.
- Quota refund on provider failure; token-budget (vs message-count) capping.
- Admin analytics over `ai_messages` token usage.
