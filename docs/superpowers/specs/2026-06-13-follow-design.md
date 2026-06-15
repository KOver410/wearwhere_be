# Follow (Brand & User) — Design

**Date:** 2026-06-13
**Scope:** SRS UC35 (Follow Brand), UC36 (Follow User) — third sub-project of Social & Community.
**Status:** Approved (design).

## 1. Goal & Context

Logged-in customers follow other users and brands; follow is idempotent and reversible. Profiles show a
denormalized follower count; the user sees who they follow; and the personalized OOTD "following" feed
(deferred from the OOTD sub-project) shows posts from followed users.

### Decomposition context

Social & Community (UC32–40): 1. Reviews (done) → 2. OOTD (done) → **3. Follow (this spec)** →
4. Moderation. Each is its own spec → plan → implementation cycle.

### Decisions (from brainstorming)

- **Two typed tables:** `user_follows` (user→user) and `brand_follows` (user→brand).
- **Profile privacy deferred:** all profiles are public this iteration; anyone may follow anyone.
  `is_private` + follow-requests are a separate future settings feature.
- **Followed-feed included:** `GET /ootd/following` returns OOTD posts from followed users. To avoid a
  Go import cycle, the OOTD repo gains a `FollowedFeed` method that JOINs `ootd_posts` to the
  `user_follows` table directly (SQL across tables, not a package import).
- **Denormalized `follower_count`** on both `users` and `brands`, updated in the follow/unfollow tx.
  A user can list who they follow; brands see only their follower count (not follower identities),
  satisfying UC35's "without consent" rule.
- **Notifications deferred:** "notify of new follower" needs a notification system that doesn't exist.

## 2. Architecture & module

New module `internal/follow/` (domain → repo → service → handler), mirroring `internal/review`/`ootd`.
The repo holds `*pgxpool.Pool` and owns the follow/unfollow transaction (junction write + counter).

- `domain/` — request/response DTOs, sentinel `*httpx.AppError`, small view structs for the
  following-lists.
- `repo/` — `Repo` interface + Postgres impl: follow/unfollow user, follow/unfollow brand,
  is-following checks, following-lists, existence checks.
- `service/` — self-follow guard, target-existence checks, orchestration.
- `handler/` + `routes.go` — `MountFollowAuthed` (all follow routes are customer-authed).

Two small cross-module additions, done as part of this sub-project:
- **OOTD module:** add `FollowedFeed` to the OOTD repo + a `Following` service method + route
  `GET /ootd/following` (authed).
- **Brand module:** surface `follower_count` (and optional `is_following`) on the public brand detail.

## 3. Data model & migrations

Three migrations, next-free on `main` (at `000039`): `000040`–`000042`.

### `user_follows` (000040)
```sql
CREATE TABLE user_follows (
    follower_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);
CREATE INDEX idx_user_follows_followee ON user_follows (followee_id);
```

### `brand_follows` (000041)
```sql
CREATE TABLE brand_follows (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    brand_id   UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, brand_id)
);
CREATE INDEX idx_brand_follows_brand ON brand_follows (brand_id);
```

### `follower_count` (000042)
```sql
ALTER TABLE users  ADD COLUMN follower_count INT NOT NULL DEFAULT 0;
ALTER TABLE brands ADD COLUMN follower_count INT NOT NULL DEFAULT 0;
```

`follower_count` is updated in the same tx as each follow/unfollow, gated by `RowsAffected()` so the
operation is idempotent (no double-count, no negative drift).

## 4. API endpoints

All follow/unfollow + list routes are customer-authed (`MountFollowAuthed`, caller chains
`RequireAuth`). The followed-feed route lives in the OOTD module's authed group.

| Method & Path | Behaviour |
|---|---|
| `POST /users/:id/follow` | Follow user `:id`. `400 CANNOT_FOLLOW_SELF` if self; `404` if target missing/deleted. Idempotent. Returns `{following:true, follower_count}`. |
| `DELETE /users/:id/follow` | Unfollow. Idempotent. Returns `{following:false, follower_count}`. |
| `POST /brands/:id/follow` | Follow brand (`:id` = brand UUID, from brand detail). `404` if missing. Returns `{following:true, follower_count}`. |
| `DELETE /brands/:id/follow` | Unfollow brand. Returns `{following:false, follower_count}`. |
| `GET /me/following/users` | Users the caller follows: `{id, name, avatar_url, follower_count}`, paginated. |
| `GET /me/following/brands` | Brands the caller follows: `{id, slug, name, logo_url, follower_count}`, paginated. |
| `GET /ootd/following` | OOTD posts from followed users, newest, paginated; same shape as the global feed (with `liked_by_me`). (OOTD module.) |

### Surfacing
- Public brand detail (`GET /brands/:slug`) gains `follower_count` and, when the request carries a valid
  token, `is_following`. (Brand module change.)
- User `follower_count` is returned in the follow/unfollow responses and the following-list items. No
  dedicated public user-profile endpoint is added (none exists today).
- Brands have no endpoint to list follower identities — only the count (UC35 "without consent").

## 5. Counts, errors & testing

### Count mechanics (in the follow/unfollow tx)
- **Follow user:** `INSERT INTO user_follows … ON CONFLICT DO NOTHING`; if a row was inserted,
  `UPDATE users SET follower_count = follower_count + 1 WHERE id = followee_id`.
- **Unfollow user:** `DELETE …`; if a row was deleted, `follower_count - 1`.
- **Brand follow/unfollow:** the same against `brand_follows` + `brands.follower_count`.
- **is_following:** `SELECT EXISTS(…)`; computed only when an authed user id is present.
- **FollowedFeed (OOTD repo):**
  `SELECT … FROM ootd_posts p JOIN user_follows uf ON uf.followee_id = p.user_id
   WHERE uf.follower_id = $1 AND p.deleted_at IS NULL AND p.status='published'
   ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`.

### Error handling (`pkg/httpx` AppError)
- Self-follow → `400 CANNOT_FOLLOW_SELF`.
- Target user/brand missing or deleted → `404`.
- Invalid `:id` (not a UUID) → `400`.
- Follow/unfollow are idempotent — already/not-following is not an error; the current state is returned.

### Testing
- **Unit (service; fake repo):** self-follow rejected; target-not-found; follow/unfollow delegate
  correctly; `is_following` reported.
- **Integration (repo; testfixtures + `testfixtures.Clean`):** follow user/brand → `follower_count++`
  and idempotent; unfollow → `--`; unique PK; DB `CHECK` rejects self-follow; following-lists;
  `is_following` EXISTS.
- **OOTD integration:** `FollowedFeed` returns only followed users' posts, excludes deleted/hidden,
  paginates.
- Add testfixtures follow-seed helpers if convenient.

## 6. Out of scope (documented exclusions)
- Profile privacy (`is_private`) + follow-requests.
- Notifications ("notify of new follower").
- Brand viewing follower identities (count only).
- Brand content in the followed-feed (UC50 not built — the feed contains only followed users' OOTD).

## 7. Branch
`feature/follow`, off `main`. Migrations `000040`–`000042` (next free; `main` at `000039`). The parked
`ai-stylist-chatbot` `000033/000034` collision is already documented.
