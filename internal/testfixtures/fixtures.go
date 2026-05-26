// Package testfixtures provides minimal row-insertion helpers for integration
// tests. Each helper requires a *testing.T (for fatal-on-error) and a DBTX
// (so callers can pass a pgx.Tx that rolls back at test end).
package testfixtures

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MustPool opens a *pgxpool.Pool from TEST_DATABASE_URL and registers a cleanup
// to close it when the test ends. It is safe to call multiple times per test —
// each call returns a fresh pool that is closed via t.Cleanup.
func MustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// GetVariantStock reads stock_qty and reserved_qty for a variant directly from
// the DB. Use the pool returned by MustPool — do NOT call inside a rolled-back
// transaction.
func GetVariantStock(t *testing.T, db DBTX, variantID uuid.UUID) (stock, reserved int) {
	t.Helper()
	err := db.QueryRow(context.Background(),
		`SELECT stock_qty, reserved_qty FROM product_variants WHERE id=$1`, variantID,
	).Scan(&stock, &reserved)
	if err != nil {
		t.Fatalf("GetVariantStock: %v", err)
	}
	return
}

// Clean truncates integration-test tables that accumulate across tests.
// It is safe to call even when the tables are empty.
func Clean(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{
		"order_items", "sub_orders", "payments", "orders",
		"cart_items", "wishlist_items", "customer_addresses",
		"product_variants", "product_images", "products",
		"brands", "users",
	}
	ctx := context.Background()
	for _, tbl := range tables {
		if _, err := pool.Exec(ctx,
			"DELETE FROM "+tbl+" WHERE TRUE"); err != nil {
			// Non-fatal — some tables may have FK constraints; best-effort.
			t.Logf("Clean: table %s: %v", tbl, err)
		}
	}
}

// DBTX is the read/write subset both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// BeginTx opens a tx that callers MUST rollback in t.Cleanup.
func BeginTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

type SeededUser struct {
	ID   uuid.UUID
	Name string
	Role string
}

// SeedUser inserts a user with given role. Email is randomized.
func SeedUser(t *testing.T, db DBTX, role string) SeededUser {
	t.Helper()
	id := uuid.New()
	email := fmt.Sprintf("u-%s@test.local", id.String()[:8])
	_, err := db.Exec(context.Background(),
		`INSERT INTO users (id, email, role, status, name)
         VALUES ($1, $2, $3, 'active', $4)`,
		id, email, role, "Test "+role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return SeededUser{ID: id, Name: "Test " + role, Role: role}
}

type SeededBrand struct {
	ID      uuid.UUID
	Slug    string
	Name    string
	OwnerID uuid.UUID
}

// SeedBrand inserts a brand. Creates an owner user if ownerID is zero.
func SeedBrand(t *testing.T, db DBTX, ownerID uuid.UUID) SeededBrand {
	t.Helper()
	if ownerID == uuid.Nil {
		ownerID = SeedUser(t, db, "brand").ID
	}
	id := uuid.New()
	slug := "brand-" + id.String()[:8]
	name := "Brand " + slug
	_, err := db.Exec(context.Background(),
		`INSERT INTO brands (id, slug, name, owner_user_id, status)
         VALUES ($1, $2, $3, $4, 'active')`,
		id, slug, name, ownerID)
	if err != nil {
		t.Fatalf("seed brand: %v", err)
	}
	return SeededBrand{ID: id, Slug: slug, Name: name, OwnerID: ownerID}
}

type SeededCategory struct {
	ID   uuid.UUID
	Slug string
}

func SeedCategory(t *testing.T, db DBTX) SeededCategory {
	t.Helper()
	id := uuid.New()
	slug := "cat-" + id.String()[:8]
	_, err := db.Exec(context.Background(),
		`INSERT INTO categories (id, slug, name) VALUES ($1, $2, $3)`,
		id, slug, "Cat "+slug)
	if err != nil {
		t.Fatalf("seed category: %v", err)
	}
	return SeededCategory{ID: id, Slug: slug}
}

type SeededStyleTag struct {
	ID   uuid.UUID
	Slug string
}

func SeedStyleTag(t *testing.T, db DBTX) SeededStyleTag {
	t.Helper()
	id := uuid.New()
	slug := "tag-" + id.String()[:8]
	_, err := db.Exec(context.Background(),
		`INSERT INTO style_tags (id, slug, name) VALUES ($1, $2, $3)`,
		id, slug, "Tag "+slug)
	if err != nil {
		t.Fatalf("seed style tag: %v", err)
	}
	return SeededStyleTag{ID: id, Slug: slug}
}

type SeededProduct struct {
	ID         uuid.UUID
	BrandID    uuid.UUID
	CategoryID uuid.UUID
	Slug       string
	Name       string
	Status     string
}

// SeedProduct inserts a product. Pass status="active" to make it visible
// in the public catalog; "draft" otherwise.
func SeedProduct(t *testing.T, db DBTX, brandID, categoryID uuid.UUID, status string) SeededProduct {
	t.Helper()
	id := uuid.New()
	slug := "p-" + id.String()[:8]
	name := "Product " + slug
	_, err := db.Exec(context.Background(),
		`INSERT INTO products (id, brand_id, category_id, slug, name, status)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		id, brandID, categoryID, slug, name, status)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return SeededProduct{
		ID: id, BrandID: brandID, CategoryID: categoryID,
		Slug: slug, Name: name, Status: status,
	}
}

// SeedVariant inserts a product_variants row with sane defaults.
func SeedVariant(t *testing.T, db DBTX, productID uuid.UUID, size, color string, price float64, stockQty int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	sku := fmt.Sprintf("SKU-%s", id.String()[:8])
	_, err := db.Exec(context.Background(),
		`INSERT INTO product_variants
           (id, product_id, sku, size, color, price, stock_qty)
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, productID, sku, size, color, price, stockQty)
	if err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	return id
}

// SeedCustomer is a thin wrapper around SeedUser with role="customer" for readability.
func SeedCustomer(t *testing.T, db DBTX) SeededUser {
	t.Helper()
	return SeedUser(t, db, "customer")
}

// SeededCartItem is the minimal info callers need after seeding a cart row.
type SeededCartItem struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	VariantID     uuid.UUID
	Qty           int
	PriceSnapshot float64
}

// SeedCartItem inserts a cart_items row. priceSnapshot must equal the variant price
// the caller passed to SeedVariant to mimic real add-to-cart flow.
func SeedCartItem(t *testing.T, db DBTX, userID, variantID uuid.UUID, qty int, priceSnapshot float64) SeededCartItem {
	t.Helper()
	id := uuid.New()
	_, err := db.Exec(context.Background(),
		`INSERT INTO cart_items (id, user_id, variant_id, qty, price_snapshot, currency_snapshot)
         VALUES ($1, $2, $3, $4, $5, 'VND')`,
		id, userID, variantID, qty, priceSnapshot)
	if err != nil {
		t.Fatalf("seed cart_item: %v", err)
	}
	return SeededCartItem{ID: id, UserID: userID, VariantID: variantID, Qty: qty, PriceSnapshot: priceSnapshot}
}

// SeedWishlistItem inserts a wishlist_items row.
func SeedWishlistItem(t *testing.T, db DBTX, userID, productID uuid.UUID) {
	t.Helper()
	_, err := db.Exec(context.Background(),
		`INSERT INTO wishlist_items (user_id, product_id) VALUES ($1, $2)`,
		userID, productID)
	if err != nil {
		t.Fatalf("seed wishlist_item: %v", err)
	}
}

// CustomerAddressOpts overrides defaults for SeedCustomerAddress.
type CustomerAddressOpts struct {
	Label          string
	RecipientName  string
	RecipientPhone string
	IsDefault      bool
}

type SeededCustomerAddress struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	IsDefault bool
}

// SeedCustomerAddress inserts a customer_addresses row with sane Vietnam defaults.
func SeedCustomerAddress(t *testing.T, db DBTX, userID uuid.UUID, opts CustomerAddressOpts) SeededCustomerAddress {
	t.Helper()
	if opts.Label == "" {
		opts.Label = "Nhà"
	}
	if opts.RecipientName == "" {
		opts.RecipientName = "Người Nhận"
	}
	if opts.RecipientPhone == "" {
		opts.RecipientPhone = "+84901234567"
	}
	id := uuid.New()
	_, err := db.Exec(context.Background(),
		`INSERT INTO customer_addresses
           (id, user_id, label, recipient_name, recipient_phone,
            address_line, ward, district, city, country, is_default)
         VALUES ($1,$2,$3,$4,$5,'123 Lê Lợi','Bến Nghé','Quận 1','TP HCM','VN',$6)`,
		id, userID, opts.Label, opts.RecipientName, opts.RecipientPhone, opts.IsDefault)
	if err != nil {
		t.Fatalf("seed customer_address: %v", err)
	}
	return SeededCustomerAddress{ID: id, UserID: userID, IsDefault: opts.IsDefault}
}
