# Moderation — Design

**Date:** 2026-06-15

**Scope:** SRS UC40 (Block User) — fourth and final sub-project of the Social & Community group.

## Context

Social & Community (UC32–40) is split into: 1. Reviews (done) → 2. OOTD (done) → 3. Follow
(done) → **4. Moderation (this spec)**. Each is its own spec → plan → implementation cycle.

The OOTD posts, OOTD comments, and product reviews tables already carry a
`status IN ('published','hidden')` column — the moderation hook reserved by the earlier
sub-projects so admins can hide content.

## Scope decisions

This sub-project deliberately ships **only the customer-facing Block User feature**. The
following SRS use cases were considered and **explicitly deferred / cut**:

- **UC39 Report Content — CUT.** No report endpoint and no `reports` table. Without an admin
  consumer the report queue has no practical value for the demo; the reportable status hook
  already exists on the content tables if reporting is revived later.
- **UC55 Moderate Content (admin hide UGC) — CUT as API.** Admins hide a violating post by
  setting `status='hidden'` directly in the database. The existing `status` column + feed
  filters (`WHERE status='published'`) already make this work with no new code.
- **UC57 Handle Reports (admin) — CUT.** Follows from UC39 being cut.

There is currently no admin route group mounted in the API; none is introduced here.

## UC40 — Block User

**Behaviour:** A customer can block another user. The effect is **one-directional content
hiding**: the blocked user's OOTD content no longer appears in any of the blocker's views.
This is the single effect chosen for this sub-project — there is **no interaction blocking**
(the blocked user may still comment on / like / follow the blocker), **no reciprocal hiding**
(the blocked user still sees the blocker's content), and **no auto-unfollow**.

### Data model

New migration `000043_create_user_blocks`:

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

The primary key gives idempotent blocks and a uniqueness guarantee; the `blocker_id` index
serves the `NOT IN (SELECT blocked_id ... WHERE blocker_id = $viewer)` subquery used by the
OOTD read filter. `ON DELETE CASCADE` cleans up rows when either account is deleted.

### `block` module

New package `internal/block/{domain,repo,service,handler}`, mirroring the existing `follow`
module's structure and conventions.

- **domain** — block entity, list-item DTO + response, errors (`ErrCannotBlockSelf`,
  `ErrUserNotFound`, reusing the shared forbidden/not-found patterns).
- **repo** (`Repo` interface + `*_pg.go`):
  - `UserExists(ctx, id) (bool, error)`
  - `Block(ctx, blocker, blocked) error` — `INSERT ... ON CONFLICT DO NOTHING` (idempotent)
  - `Unblock(ctx, blocker, blocked) error` — idempotent delete
  - `ListBlocked(ctx, blocker, limit, offset) ([]BlockedUserItem, int, error)`
- **service** — rejects self-block (`blocker == target`), verifies the target user exists
  before inserting; orchestrates list pagination.
- **handler + routes** — customer-authed endpoints, mounted in the existing `reviewsAuthed`
  group in `cmd/api/main.go` (same group as follow/ootd authed routes):
  - `POST   /users/:id/block` → `{ "blocked": true }`
  - `DELETE /users/:id/block` → 204
  - `GET    /me/blocks` → paginated list of users the caller has blocked

### Applying the hide-their-content effect to OOTD reads (Approach A — SQL subquery)

The block filter is applied entirely inside the OOTD repository's read queries. Every OOTD
read path that can surface another user's content gains the clause:

```sql
AND <author_col> NOT IN (SELECT blocked_id FROM user_blocks WHERE blocker_id = $viewer)
```

When the viewer is a guest (`viewerID == uuid.Nil`), the subquery returns no rows and
`NOT IN (empty)` passes everything — so no conditional branching is needed; the same query
serves authenticated and anonymous callers.

Touch points:

| Read path | Method | Change |
|-----------|--------|--------|
| Feed `/ootd` | `FeedList` | add `viewerID` param + subquery on `ootd_posts.user_id` |
| Following `/ootd-following` | `FollowedFeed` | already has `viewerID`; add subquery |
| By user `/users/:id/ootd` | `ListByUser` | add `viewerID` param + subquery |
| Comments `/ootd/:id/comments` | `ListComments` | add `viewerID` param + subquery on `ootd_comments.user_id`; thread `viewerID` through `service.ListComments` + handler |
| Detail `/ootd/:id` | `service.GetPost` | new repo `IsBlocked(blocker, blocked) (bool, error)`; if `viewer != Nil` and blocked → return `ErrPostNotFound` |

**Why Detail is handled separately:** `repo.GetPost` is reused for internal ownership and
like/comment checks (`UpdateCaption`, `DeletePost`, `Like`, `AddComment`). Those must **not**
be block-filtered, so `repo.GetPost` keeps its current signature and the Detail-only filter
lives in `service.GetPost` via the dedicated `IsBlocked` lookup.

**Module-boundary note:** the OOTD repo references the `user_blocks` table directly (the
`block` module owns writes to it). This DB-layer coupling is consistent with existing
cross-table reads in the codebase (e.g. review aggregates writing to `products`) and was
chosen over injecting a block reader for leanness and to keep feed reads to a single query.

## Error handling

- Block a non-existent user → `ErrUserNotFound` (404).
- Block self → `ErrCannotBlockSelf` (400/422 per the shared error convention).
- Re-blocking / un-blocking when no row exists → idempotent success (no error).
- Viewing a blocked user's post by id → 404 `POST_NOT_FOUND` (indistinguishable from a
  genuinely missing post, by design).

## Testing

- **block repo (pg):** idempotent block, unblock, self-block excluded by CHECK, list +
  pagination, cascade on user delete.
- **block service:** self-block rejected, missing-target rejected, happy paths.
- **block handler:** route wiring, auth required, status codes, invalid id.
- **ootd (regression):** a blocked user's post is absent from Feed, Following, ByUser; their
  comment is absent from the comment list; Detail of their post returns 404 for the blocker
  but remains visible to other users and to guests.

## Out of scope (recap)

Reporting (UC39), admin moderation API (UC55/UC57), interaction blocking, reciprocal hiding,
auto-unfollow on block, blocking brands. Admin content takedown is done by editing
`status` in the database directly.
