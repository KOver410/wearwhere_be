# Promo Code (Mã giảm giá) Implementation Plan

> **Status (2026-06-19): IMPLEMENTED on branch `feature/promo-codes`.** Tasks 1–8 done. `go build`/`go vet`/unit tests pass. Integration tests (`//go:build integration`) are written and compile, but were not executed here (no `TEST_DATABASE_URL`/Postgres/migrate CLI in this environment) — run them + apply migrations 47–49 in CI/local.

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking. Implement task-by-task; run quality gates after each task that changes code.

**Goal:** Cho phép khách hàng nhập **mã giảm giá** lúc checkout để được giảm tiền đơn hàng. MVP hỗ trợ: giảm theo **phần trăm** (có trần tùy chọn) và **số tiền cố định**; ràng buộc **hạn sử dụng** (`starts_at`/`ends_at`), **giá trị đơn tối thiểu**, và **mỗi user dùng 1 lần**. Admin quản lý mã qua CRUD.

**Architecture:** Module mới `internal/promo/` (domain → repo → service → handler), mirror `internal/wishlist`/`order`. Repo dùng `DBTX` (pool hoặc `pgx.Tx`) để service order có thể validate + ghi redemption **trong cùng transaction** đặt hàng. Giảm giá là **điều chỉnh cấp đơn hàng** (order-level): `grand_total = subtotal + shipping − discount`; sub_orders giữ nguyên total theo brand (promo do nền tảng tài trợ, không phân bổ theo brand cho MVP).

**Tech Stack:** Go, Gin, pgx/v5, PostgreSQL, golang-migrate. Patterns từ `internal/wishlist` (handler/service/repo) và `internal/order` (repo tx + DBTX).

**Branch:** `feature/promo-codes` (tạo off `main`).

> **Enforcement "mỗi user 1 lần":** ràng buộc DB `UNIQUE(promo_code_id, user_id)` trên `promo_redemptions` + khóa hàng promo `FOR UPDATE` trong tx → chống dùng lại & race 2 đơn đồng thời. Multi-use-per-user là việc tương lai (cần đổi unique key + đếm).
> **Customer validate trước checkout:** không làm endpoint riêng — `GET /me/checkout/preview?promo_code=XXX` đã trả `discount_vnd` + `promo_error`, FE gọi preview để kiểm mã.

---

## File Structure

**New `internal/promo/`:**
- `domain/promo.go` — `PromoCode`, `DiscountType` enum, `ComputeDiscount`, `ValidateResult`
- `domain/errors.go` — AppErrors: not-found / inactive / not-started / expired / min-order / already-used / not-applicable
- `domain/dto.go` — admin CRUD DTOs
- `repo/repo.go` — `DBTX` + `PromoRepo` interface + sentinel errors
- `repo/promo_pg.go` — Postgres impl (read/lock/redeem + admin CRUD)
- `repo/promo_pg_test.go` — integration tests
- `service/service.go` — `Validate` (read-only), `ValidateTx`+`RedeemTx` (in-tx), admin ops
- `service/service_test.go`
- `handler/handler.go` — admin CRUD HTTP
- `handler/routes.go` — `MountAdmin`
- `handler/handler_test.go`

**Modified:**
- `db/migrations/000047`–`000049`
- `internal/order/domain/order.go` (`Order.DiscountVND`, `Order.PromoCode`), `domain/dto.go` (`PlaceOrderReq.PromoCode`; `OrderResp`/`CheckoutPreviewResp` discount fields)
- `internal/order/repo/order_pg.go` (insert + scan new cols)
- `internal/order/service/order_service.go` (promo port, apply in tx), `checkout_service.go` (promo validator in preview)
- `internal/order/handler/handler.go` (promo_code query param; map promo AppErrors)
- `internal/order/service/order_service_test.go` (`buildSvc`), `checkout_service_test.go` (NewCheckoutService calls)
- `cmd/api/main.go` (wire promo repo/service/handler; admin group), `cmd/api/main_test.go` if route-count asserted

> **Migration numbers:** `main` head at `000046`. Next free: `000047`–`000049`.

---

## Task 1: Migrations

- [ ] **Step 1:** `000047_create_promo_codes.up.sql`
```sql
CREATE TYPE promo_discount_type AS ENUM ('percentage', 'fixed');

CREATE TABLE promo_codes (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code                CITEXT NOT NULL UNIQUE,
    description         TEXT,
    discount_type       promo_discount_type NOT NULL,
    discount_value      BIGINT NOT NULL CHECK (discount_value > 0), -- percent (1..100) OR VND amount
    max_discount_vnd    BIGINT CHECK (max_discount_vnd IS NULL OR max_discount_vnd > 0), -- cap for percentage
    min_order_value_vnd BIGINT NOT NULL DEFAULT 0 CHECK (min_order_value_vnd >= 0),
    starts_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at             TIMESTAMPTZ,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (discount_type <> 'percentage' OR discount_value BETWEEN 1 AND 100),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);
CREATE INDEX idx_promo_codes_active ON promo_codes (is_active, starts_at, ends_at);
```
`down`: `DROP TABLE IF EXISTS promo_codes; DROP TYPE IF EXISTS promo_discount_type;`

- [ ] **Step 2:** `000048_create_promo_redemptions.up.sql`
```sql
CREATE TABLE promo_redemptions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    promo_code_id UUID NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    discount_vnd  BIGINT NOT NULL CHECK (discount_vnd >= 0),
    redeemed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (promo_code_id, user_id)   -- one redemption per user (MVP)
);
CREATE INDEX idx_promo_redemptions_user ON promo_redemptions (user_id);
```
`down`: `DROP TABLE IF EXISTS promo_redemptions;`

- [ ] **Step 3:** `000049_add_order_discount.up.sql`
```sql
ALTER TABLE orders ADD COLUMN discount_vnd BIGINT NOT NULL DEFAULT 0 CHECK (discount_vnd >= 0);
ALTER TABLE orders ADD COLUMN promo_code   CITEXT;
```
`down`: `ALTER TABLE orders DROP COLUMN IF EXISTS discount_vnd; ALTER TABLE orders DROP COLUMN IF EXISTS promo_code;`

- [ ] **Step 4:** Apply on test DB + verify up/down/up clean. Leave DB at 49.

---

## Task 2: promo domain

- [ ] `DiscountType` enum (`percentage`/`fixed`); `PromoCode` struct mirrors columns.
- [ ] `ComputeDiscount(subtotalVND) int64`: percentage → `subtotal*value/100`, cap at `MaxDiscountVND` if set; fixed → `value`; clamp at `subtotal` (never below 0; shipping not discounted).
- [ ] `ValidateResult{ PromoID, Code, DiscountVND, DiscountType, DiscountValue }`.
- [ ] Errors (`httpx.NewAppError`): `ErrPromoNotFound`(404), `ErrPromoExpired`(422), `ErrPromoNotStarted`(422), `ErrPromoMinOrder`(422), `ErrPromoAlreadyUsed`(409), `ErrPromoNotApplicable`(422). Inactive/unknown both collapse to `ErrPromoNotFound` (don't leak existence).
- [ ] DTOs: `CreatePromoReq`, `UpdatePromoReq`, `PromoResp`, list resp.

## Task 3: promo repo

- [ ] `DBTX` interface (Exec/Query/QueryRow); `PromoRepo`:
  - `GetActiveByCode(ctx, db, code)` / `GetActiveByCodeForUpdate(ctx, db, code)` (`FOR UPDATE`)
  - `HasRedeemed(ctx, db, promoID, userID) (bool,error)`
  - `InsertRedemption(ctx, db, promoID, userID, orderID, discountVND) error` → maps `23505` to `ErrAlreadyRedeemed`
  - Admin: `Create`, `Update`, `GetByID`, `List(page,pageSize,activeOnly)`
- [ ] `nil db` → fall back to pool (mirror order_pg pattern).

## Task 4: promo service

- [ ] `Validate(ctx, code, userID, subtotal) (*ValidateResult, error)` — read-only (pool): trim/normalize code; empty → `(nil,nil)`; load active-by-code; run `check()`; check `HasRedeemed`.
- [ ] `ValidateTx(ctx, db, code, userID, subtotal) (promoID uuid.UUID, discountVND int64, err error)` — same but `GetActiveByCodeForUpdate(db)` (locks row in caller's tx) + `HasRedeemed(db)`. Empty code → `(uuid.Nil, 0, nil)`.
- [ ] `RedeemTx(ctx, db, promoID, userID, orderID, discountVND) error` — `InsertRedemption`; surface `ErrPromoAlreadyUsed` on unique violation (race).
- [ ] Shared `check(p, now, subtotal, redeemed)`: inactive/nil→NotFound; `now<starts`→NotStarted; `ends!=nil && now>ends`→Expired; `subtotal<min`→MinOrder; redeemed→AlreadyUsed; `ComputeDiscount<=0`→NotApplicable.
- [ ] Admin: `CreateCode`, `UpdateCode`, `GetCode`, `ListCodes` with input validation (percentage value 1..100, value>0).

## Task 5: promo handler + routes (admin)

- [ ] `POST /admin/promo-codes`, `GET /admin/promo-codes`, `GET /admin/promo-codes/:id`, `PATCH /admin/promo-codes/:id`. Use `httpx` helpers + `ErrorFromApp`.
- [ ] `MountAdmin(rg, h)`.

## Task 6: order integration

- [ ] **domain:** `Order.DiscountVND int64`, `Order.PromoCode string`; `PlaceOrderReq.PromoCode string` (`binding:"max=40"`); add `DiscountVND`/`PromoCode` to `OrderResp` (omitempty code) and `CheckoutPreviewResp` (`DiscountVND`, `PromoCode`, `PromoApplied`, `PromoError`).
- [ ] **order_pg:** add `discount_vnd, promo_code` to `orderCols`, `scanOrder`, and `Create` INSERT (`NULLIF(promo_code,'')`).
- [ ] **order_service:** define `promoPort` interface (`ValidateTx`/`RedeemTx` over `promorepo.DBTX`); add nil-able field + constructor param. In `PlaceOrder`, after Step 7 (subtotal/shipping) & before order Create: if `req.PromoCode != ""` and port set → `ValidateTx(tx,...)` → set `order.DiscountVND`, `order.PromoCode`, `grandTotal -= discount`. After order Create → `RedeemTx(tx, promoID, userID, order.ID, discount)`. Errors roll back tx.
- [ ] **checkout_service:** define `promoValidator` (`Validate`); nil-able field + ctor param. `Preview(ctx, userID, addressID, promoCode)`: validate against `subtotalAll`; on success set discount + `GrandTotal -= discount`; on error set `PromoError` (string) and discount 0 (don't fail preview).
- [ ] **handler:** read `promo_code` query in `PreviewCheckout`; pass through. In `PlaceOrder` switch, add `errors.As(err, *httpx.AppError)` branch → render `appErr.Status/Code/Message` (surfaces PROMO_* codes).
- [ ] **orderToResp:** map discount + promo_code.

## Task 7: wiring

- [ ] `cmd/api/main.go`: `promoRepo := promorepo.NewPromoPG(pgPool)`; `promoSvc := promoservice.New(promoRepo)`; pass to `NewCheckoutService(..., promoSvc)` and `NewOrderService(..., promoSvc)`; `adminGroup := v1.Group("/admin", RequireAuth, RequireRole(RoleAdmin))`; `promohandler.MountAdmin(adminGroup, promohandler.New(promoSvc))`.
- [ ] Update `buildSvc` (order test) + 6 `NewCheckoutService` test calls (pass `nil` promo).

## Task 8: tests + gates

- [ ] promo repo/service/handler unit + integration tests (expired, min-order, already-used %/fixed, clamp, race two concurrent orders, cap).
- [ ] order/checkout: place with valid % and fixed code → discount in totals + redemption row; second order same user same code → 409; preview with bad code → warning.
- [ ] `go build ./... && go test ./...`; `go vet`. Migrate test DB up to 49.

---

## Edge cases / decisions
- Discount clamps at `subtotal` → customer always pays ≥ shipping. (Extreme: 100% + free shipping → grand_total 0; acceptable for MVP, note for PayOS amount>0 if ever combined.)
- Promo's own `min_order_value` is **independent** of platform `MinOrderValueVND` (50k) — both enforced.
- Inactive/expired/unknown disambiguation: unknown+inactive → 404; expired/not-started → 422 (so FE can message "mã đã hết hạn").
- Validate re-runs inside `PlaceOrder` tx (never trust preview); redemption insert in same tx as order.
