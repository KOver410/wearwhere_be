# OOTD (Outfit of the Day) — Design

**Date:** 2026-06-13
**Scope:** SRS UC32 (Post OOTD), UC33 (Like OOTD), UC34 (Comment OOTD) — second sub-project of the Social & Community group.
**Status:** Approved (design).

## 1. Goal & Context

Customers post outfit photos with optional caption and tagged products ("shop the look"); anyone can
browse the feed and a user's posts; logged-in customers like and comment on posts. This is the Go
backend repo; map/photo rendering is the client's job — the backend stores photos (via the existing
`storage.Storage` backend) and serves post data.

### Decomposition context

Social & Community (UC32–40) is split into: 1. Reviews (done) → **2. OOTD (this spec)** → 3. Follow
(brand/user) → 4. Moderation (report/block). Each is its own spec → plan → implementation cycle.

### Decisions (from brainstorming)

- **Photos included** now: multipart upload reusing the existing `storage.Storage` backend (same
  pattern as product image upload). A post has 1–10 photos.
- **Product tagging** included: a post links to 1+ products via a junction table.
- **Edit/delete:** owners soft-delete their own posts and comments; owners may edit a post's caption.
  Comments are immutable (delete + re-add to change).
- **Feed scope now:** global public feed (newest, paginated) + per-user posts. Personalized
  "followed users' feed" is deferred to the Follow sub-project.
- **Likes** are idempotent `POST`/`DELETE` (not a toggle), with a denormalized `like_count`.
- **Comments** are flat (no threading), max 500 chars, emoji allowed.
- **Moderation hook only:** posts/comments publish immediately (`status='published'`); a `status`
  column lets the future Moderation sub-project hide them. Profanity auto-moderation, inappropriate-
  content blocking, and reporting are deferred to Moderation.
- **Notifications deferred:** UC34's "author notified" requires a notification system that doesn't
  exist yet — out of scope.
- **Image compression deferred:** UC32's "image >10MB compress" — we enforce a max size and reject
  oversize files (no server-side compression).

## 2. Architecture & module

New module `internal/ootd/` (domain → repo → service → handler), mirroring `internal/review`. The repo
owns transactions for multi-table/counter writes (post + product tags; like + counter; comment +
counter; soft-deletes + counter). The service holds the repo plus the `storage.Storage` backend and
the upload limits (`cfg.Storage.AllowedMIMEs`, `cfg.Storage.MaxFileSize`) for photo handling.

- `domain/` — `Post`, `Comment` entities, view structs (with author name, tagged products, liked-by-me),
  DTOs, sentinel `*httpx.AppError`.
- `repo/` — `Repo` interface + Postgres impl (pool-owned, transactional writes + counter updates).
- `service/` — validation, ownership, photo upload orchestration, like/comment logic.
- `handler/` + `routes.go` — `MountOOTDPublic` (reads) + `MountOOTDAuthed` (writes).

## 3. Data model & migrations

Four migrations, next-free on this branch off `main` (which is at `000035`): `000036`–`000039`.

### `ootd_posts` (000036)
```sql
CREATE TABLE ootd_posts (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    caption       TEXT,
    photo_urls    TEXT[] NOT NULL CHECK (array_length(photo_urls,1) BETWEEN 1 AND 10),
    status        TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    like_count    INT  NOT NULL DEFAULT 0,
    comment_count INT  NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX idx_ootd_posts_feed ON ootd_posts (created_at DESC) WHERE deleted_at IS NULL AND status='published';
CREATE INDEX idx_ootd_posts_user ON ootd_posts (user_id, created_at DESC) WHERE deleted_at IS NULL;
```

### `ootd_post_products` (000037)
```sql
CREATE TABLE ootd_post_products (
    post_id    UUID NOT NULL REFERENCES ootd_posts(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, product_id)
);
```

### `ootd_likes` (000038)
```sql
CREATE TABLE ootd_likes (
    post_id    UUID NOT NULL REFERENCES ootd_posts(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id)
);
```

### `ootd_comments` (000039)
```sql
CREATE TABLE ootd_comments (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id    UUID NOT NULL REFERENCES ootd_posts(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_ootd_comments_post ON ootd_comments (post_id, created_at) WHERE deleted_at IS NULL AND status='published';
```

`like_count` / `comment_count` are denormalized on `ootd_posts` and updated in the same transaction as
each like/unlike/comment/comment-delete.

## 4. API endpoints

Public reads via `MountOOTDPublic`; customer-authed writes via `MountOOTDAuthed` (caller chains
`RequireAuth`). Ownership checked in the service.

| Method & Path | Auth | Behaviour |
|---|---|---|
| `POST /ootd` | Customer | Multipart: `photos` (1–10 files), `caption?`, `product_ids` (repeated form field, optional). Uploads photos via `storage.Put`, then in one tx inserts the post + product tags. `201 {id}`. |
| `GET /ootd` | Public | Global feed: `published`, non-deleted, newest, paginated (`page`/`limit`). Each item: author name, photo URLs, caption, `like_count`, `comment_count`, tagged-product summaries, `liked_by_me`. |
| `GET /ootd/:id` | Public | Single post detail (same shape as a feed item). `404` if not found/hidden/deleted. |
| `GET /users/:id/ootd` | Public | A user's posts, newest, paginated. |
| `PATCH /ootd/:id` | Customer (owner) | Edit `caption` (JSON). `403` non-owner, `404` not found. |
| `DELETE /ootd/:id` | Customer (owner) | Soft-delete the post. |
| `POST /ootd/:id/like` | Customer | Like (idempotent; INSERT … ON CONFLICT DO NOTHING; `like_count++` only when a row is actually inserted). |
| `DELETE /ootd/:id/like` | Customer | Unlike (idempotent; `like_count--` only when a row is actually deleted). |
| `POST /ootd/:id/comments` | Customer | Add comment (JSON `body`, 1–500 chars). `comment_count++`. `201 {id}`. |
| `GET /ootd/:id/comments` | Public | List `published` comments, newest-first or oldest-first (oldest-first default), paginated. |
| `DELETE /ootd/comments/:id` | Customer (owner) | Soft-delete own comment; `comment_count--`. |

- `liked_by_me`: computed only when the request carries a valid Bearer token; for guests it is `false`.
  The feed route is public but optionally reads the authenticated user id if present.

## 5. Photo upload

Reuses `storage.Storage` (`Put(ctx, Object{Key,ContentType,Size}, reader) → url`) and the config limits,
exactly like `internal/product` image upload:
- `POST /ootd` is multipart; field `photos`. Service validates: 1–10 files; each MIME ∈
  `cfg.Storage.AllowedMIMEs`; each size ≤ `cfg.Storage.MaxFileSize`.
- Upload each file with key `ootd/<post-id>/<filename>` → collect URLs.
- Photos upload to storage BEFORE opening the DB tx; if the tx then fails, best-effort `storage.Delete`
  the uploaded objects (same rollback approach as product image upload).
- No server-side compression (deferred); oversize files are rejected with `400`.

## 6. Counts, errors & testing

### Denormalized counts (in the write tx)
- **Like:** `INSERT INTO ootd_likes … ON CONFLICT DO NOTHING`; if a row was inserted, `like_count++`.
  **Unlike:** `DELETE`; if a row was deleted, `like_count--`. Idempotent; count always matches.
- **Comment:** insert + `comment_count++`. **Comment soft-delete:** set `deleted_at` + `comment_count--`.
- **Post soft-delete:** counts ride with the post (no separate update).

### Error handling (`pkg/httpx` AppError)
- No files / >10 files / bad MIME / oversize → `400`.
- Comment `body` empty or >500 → `400`; caption >2000 → `400`.
- Post/comment not found (or hidden/deleted) → `404`; not owner → `403`.

### Testing
- **Unit (service; fake repo + fake storage):** photo count/MIME/size validation; caption/body length;
  ownership on edit/delete; like idempotency drives `like_count` correctly; comment count.
- **Integration (repo; testfixtures + `testfixtures.Clean` teardown, as in reviews):** create post +
  tags; feed + per-user list + pagination; like idempotency (`++`/`--`, unique PK); comment add +
  soft-delete + count; `hidden`/`deleted` excluded from feed and comment list.
- Add a testfixtures helper to seed a post if convenient.

## 7. Out of scope (documented exclusions)
- Server-side image compression.
- Notifications ("author notified").
- Profanity auto-moderation, inappropriate-content blocking, reporting (Moderation sub-project).
- Personalized "followed users" feed (Follow sub-project).
- Comment threading/replies.

## 8. Branch
`feature/ootd`, off `main`. Migrations `000036`–`000039` (next free on `main`, at `000035`). The parked
`ai-stylist-chatbot` branch's `000033/000034` collision is already documented for eventual integration.
