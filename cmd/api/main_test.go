//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	authdomain "github.com/wearwhere/wearwhere_be/internal/auth/domain"
	authhandler "github.com/wearwhere/wearwhere_be/internal/auth/handler"
	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	authservice "github.com/wearwhere/wearwhere_be/internal/auth/service"
	brandhandler "github.com/wearwhere/wearwhere_be/internal/brand/handler"
	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	brandservice "github.com/wearwhere/wearwhere_be/internal/brand/service"
	carthandler "github.com/wearwhere/wearwhere_be/internal/cart/handler"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	cartservice "github.com/wearwhere/wearwhere_be/internal/cart/service"
	"github.com/wearwhere/wearwhere_be/internal/config"
	customeraddrhandler "github.com/wearwhere/wearwhere_be/internal/customeraddr/handler"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	customeraddrservice "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
	producthandler "github.com/wearwhere/wearwhere_be/internal/product/handler"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	productservice "github.com/wearwhere/wearwhere_be/internal/product/service"
	jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
	authvalidator "github.com/wearwhere/wearwhere_be/internal/shared/validator"
	wishlisthandler "github.com/wearwhere/wearwhere_be/internal/wishlist/handler"
	wishlistrepo "github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
	wishlistservice "github.com/wearwhere/wearwhere_be/internal/wishlist/service"
)

func buildTestServer(t *testing.T, pool *pgxpool.Pool, storageBackend storage.Storage) (*httptest.Server, *jwtsvc.Issuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authvalidator.RegisterWithGin()

	jwtIssuer := jwtsvc.NewIssuer("test-secret", 15*time.Hour)
	userRepo := authrepo.NewUserPG(pool)
	sessionRepo := authrepo.NewSessionPG(pool)

	// Auth (minimal — login flow only; nil OTP/attempt stores are fine for E2E seeding)
	tokenSvc := authservice.NewTokenService(jwtIssuer, sessionRepo, 24*time.Hour)
	authSvc := authservice.NewAuthService(userRepo, nil, tokenSvc, nil, config.LimitConfig{})

	brandRepo := brandrepo.NewBrandPG(pool)
	addressRepo := brandrepo.NewAddressPG(pool)
	brandSvc := brandservice.New(brandRepo, addressRepo)

	productRepo := productrepo.NewProductPG(pool)
	variantRepo := productrepo.NewVariantPG(pool)
	imageRepo := productrepo.NewImagePG(pool)
	categoryRepo := productrepo.NewCategoryPG(pool)
	styleTagRepo := productrepo.NewStyleTagPG(pool)
	catalogRepo := productrepo.NewCatalogPG(pool)

	productSvc := productservice.New(productRepo, variantRepo, imageRepo,
		categoryRepo, styleTagRepo, storageBackend,
		[]string{"image/jpeg", "image/png", "image/webp"}, 5*1024*1024)
	catalogSvc := productservice.NewCatalog(catalogRepo, productRepo)

	r := gin.New()
	r.Use(gin.Recovery())
	v1 := r.Group("/api/v1", authmw.OptionalAuth(jwtIssuer))

	authhandler.Mount(v1, &authhandler.Deps{
		Auth:      authhandler.NewAuthHandler(authSvc),
		JWTIssuer: jwtIssuer,
	})

	brandGroup := v1.Group("/brand/me",
		authmw.RequireAuth(jwtIssuer),
		authmw.RequireRole(authdomain.RoleBrand),
		brandmw.BrandContext(brandRepo),
	)
	brandhandler.Mount(brandGroup, &brandhandler.Deps{
		Brand:   brandhandler.NewBrandHandler(brandSvc),
		Address: brandhandler.NewAddressHandler(brandSvc),
	})
	producthandler.MountBrandProducts(brandGroup, producthandler.NewBrandProductHandler(productSvc))
	producthandler.MountCatalog(v1, producthandler.NewCatalogHandler(catalogSvc, categoryRepo, styleTagRepo, brandRepo))
	brandhandler.MountBrandsPublic(v1, brandhandler.NewBrandsPublicHandler(brandSvc))

	// ── Sprint 2: customer-side modules wired under /me ──
	customeraddrRepo := customeraddrrepo.NewAddressPG(pool)
	wishlistRepo := wishlistrepo.NewWishlistPG(pool)
	cartRepo := cartrepo.NewCartPG(pool)

	customerAddrSvc := customeraddrservice.New(customeraddrRepo)
	wishlistSvc := wishlistservice.New(wishlistRepo, productRepo)
	cartSvc := cartservice.New(cartRepo, variantRepo)

	customerGroup := v1.Group("/me",
		authmw.RequireAuth(jwtIssuer),
		authmw.RequireRole(authdomain.RoleCustomer),
	)
	customeraddrhandler.Mount(customerGroup, customeraddrhandler.New(customerAddrSvc))
	wishlisthandler.Mount(customerGroup, wishlisthandler.New(wishlistSvc))
	carthandler.Mount(customerGroup, carthandler.New(cartSvc))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, jwtIssuer
}

func issueTokenForOwner(t *testing.T, jwtIssuer *jwtsvc.Issuer, ownerID string) string {
	t.Helper()
	tok, _, err := jwtIssuer.IssueAccess(ownerID, string(authdomain.RoleBrand), "owner@e2e.test")
	require.NoError(t, err)
	return tok
}

func issueTokenForCustomer(t *testing.T, jwtIssuer *jwtsvc.Issuer, customerID string) string {
	t.Helper()
	tok, _, err := jwtIssuer.IssueAccess(customerID, string(authdomain.RoleCustomer), "customer@e2e.test")
	require.NoError(t, err)
	return tok
}

func TestE2E_BrandCreatesProduct_AppearsInCatalog(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Seed: brand owner, brand, category — permanent rows (server uses pool, not tx).
	// Clean up at end.
	ctx := context.Background()
	var ownerID, brandID, categoryID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (email, role, status, name)
         VALUES ('e2e-owner@test.local', 'brand', 'active', 'E2E Owner')
         RETURNING id`).Scan(&ownerID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO brands (slug, name, owner_user_id, status)
         VALUES ('e2e-brand', 'E2E Brand', $1, 'active') RETURNING id`,
		ownerID).Scan(&brandID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO categories (slug, name) VALUES ('e2e-cat', 'E2E Category') RETURNING id`).
		Scan(&categoryID))

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM product_images WHERE product_id IN
          (SELECT id FROM products WHERE brand_id=$1)`, brandID)
		pool.Exec(ctx, `DELETE FROM product_variants WHERE product_id IN
          (SELECT id FROM products WHERE brand_id=$1)`, brandID)
		pool.Exec(ctx, `DELETE FROM products WHERE brand_id=$1`, brandID)
		pool.Exec(ctx, `DELETE FROM brand_addresses WHERE brand_id=$1`, brandID)
		pool.Exec(ctx, `DELETE FROM brands WHERE id=$1`, brandID)
		pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, ownerID)
		pool.Exec(ctx, `DELETE FROM categories WHERE id=$1`, categoryID)
	})

	backend := storage.NewLocal(t.TempDir(), "http://test/uploads")
	srv, jwtIssuer := buildTestServer(t, pool, backend)
	token := issueTokenForOwner(t, jwtIssuer, ownerID)

	// 1. Create product
	body := fmt.Sprintf(`{"name":"E2E Áo Thun","category_id":"%s"}`, categoryID)
	createResp := postJSON(t, srv.URL+"/api/v1/brand/me/products", token, body, http.StatusCreated)
	productID := createResp["product"].(map[string]any)["id"].(string)

	// 2. Add a variant
	variantBody := `{"sku":"E2E-001","size":"M","color":"White","price":250000,"stock_qty":10}`
	_ = postJSON(t, srv.URL+"/api/v1/brand/me/products/"+productID+"/variants", token, variantBody, http.StatusCreated)

	// 3. Upload an image (multipart)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "tiny.jpg")
	// Minimal valid JPEG bytes — enough for http.DetectContentType
	fw.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 0xff, 0xd9})
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/brand/me/products/"+productID+"/images", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "image upload should succeed")

	// 4. Publish (status draft → active)
	patchJSON(t, srv.URL+"/api/v1/brand/me/products/"+productID, token, `{"status":"active"}`, http.StatusNoContent)

	// 5. Public list (no auth) — search by name fragment
	list := getJSON(t, srv.URL+"/api/v1/products?q=ao+thun", "", http.StatusOK)
	items := list["items"].([]any)
	require.GreaterOrEqual(t, len(items), 1)

	// 6. Public detail
	productSlug := items[0].(map[string]any)["slug"].(string)
	detail := getJSON(t, srv.URL+"/api/v1/brands/e2e-brand/products/"+productSlug, "", http.StatusOK)
	prod := detail["product"].(map[string]any)
	variants := prod["variants"].([]any)
	images := prod["images"].([]any)
	require.Len(t, variants, 1)
	require.Len(t, images, 1)
}

func TestE2E_CustomerShoppingFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ctx := context.Background()

	// Seed: brand owner, brand, category, ACTIVE product with variant, customer.
	var ownerID, brandID, categoryID, productID, variantID, customerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (email, role, status, name)
         VALUES ('e2e-s2-owner@test.local', 'brand', 'active', 'Owner')
         RETURNING id`).Scan(&ownerID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO brands (slug, name, owner_user_id, status)
         VALUES ('e2e-s2-brand', 'E2E S2 Brand', $1, 'active')
         RETURNING id`, ownerID).Scan(&brandID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO categories (slug, name) VALUES ('e2e-s2-cat', 'E2E S2 Cat')
         RETURNING id`).Scan(&categoryID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO products (brand_id, category_id, slug, name, status)
         VALUES ($1, $2, 'e2e-s2-prod', 'E2E S2 Áo Thun', 'active')
         RETURNING id`, brandID, categoryID).Scan(&productID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO product_variants (product_id, sku, size, color, price, stock_qty)
         VALUES ($1, 'E2E-S2-001', 'M', 'Black', 199000, 100)
         RETURNING id`, productID).Scan(&variantID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (email, role, status, name)
         VALUES ('e2e-s2-customer@test.local', 'customer', 'active', 'Customer')
         RETURNING id`).Scan(&customerID))

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM cart_items WHERE user_id=$1`, customerID)
		pool.Exec(ctx, `DELETE FROM wishlist_items WHERE user_id=$1`, customerID)
		pool.Exec(ctx, `DELETE FROM customer_addresses WHERE user_id=$1`, customerID)
		pool.Exec(ctx, `DELETE FROM product_variants WHERE product_id=$1`, productID)
		pool.Exec(ctx, `DELETE FROM products WHERE id=$1`, productID)
		pool.Exec(ctx, `DELETE FROM brands WHERE id=$1`, brandID)
		pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, customerID)
		pool.Exec(ctx, `DELETE FROM categories WHERE id=$1`, categoryID)
	})

	backend := storage.NewLocal(t.TempDir(), "http://test/uploads")
	srv, jwtIssuer := buildTestServer(t, pool, backend)
	token := issueTokenForCustomer(t, jwtIssuer, customerID)

	// === ADDRESSES ===
	// First address → auto-default.
	addr1Body := `{"label":"Nhà","recipient_name":"Nguyen Van X","recipient_phone":"+84901234567","address_line":"1 A","ward":"P 1","district":"Q 1","city":"TP HCM"}`
	addr1 := postJSON(t, srv.URL+"/api/v1/me/addresses", token, addr1Body, http.StatusCreated)
	require.True(t, addr1["is_default"].(bool))

	// Second address with explicit is_default=true → first should be auto-unset.
	addr2Body := `{"label":"Office","recipient_name":"Nguyen Van Y","recipient_phone":"+84901234568","address_line":"2 B","ward":"P 2","district":"Q 2","city":"TP HCM","is_default":true}`
	addr2 := postJSON(t, srv.URL+"/api/v1/me/addresses", token, addr2Body, http.StatusCreated)
	require.True(t, addr2["is_default"].(bool))

	listAddrs := getJSON(t, srv.URL+"/api/v1/me/addresses", token, http.StatusOK)
	items := listAddrs["items"].([]any)
	require.Len(t, items, 2)
	var defaults int
	for _, it := range items {
		if it.(map[string]any)["is_default"].(bool) {
			defaults++
		}
	}
	require.Equal(t, 1, defaults)

	// === WISHLIST ===
	// Idempotent add (twice → both 200).
	_ = postJSON(t, srv.URL+"/api/v1/me/wishlist/"+productID, token, "", http.StatusOK)
	_ = postJSON(t, srv.URL+"/api/v1/me/wishlist/"+productID, token, "", http.StatusOK)
	// List.
	wl := getJSON(t, srv.URL+"/api/v1/me/wishlist?page=1&limit=24", token, http.StatusOK)
	require.GreaterOrEqual(t, len(wl["items"].([]any)), 1)
	// Contains.
	contains := getJSON(t, srv.URL+"/api/v1/me/wishlist/contains?product_ids="+productID, token, http.StatusOK)
	inMap := contains["in_wishlist"].(map[string]any)
	require.True(t, inMap[productID].(bool))

	// === CART ===
	// Add qty=2 then qty=3 → UPSERT increment → qty=5.
	addBody1 := fmt.Sprintf(`{"variant_id":"%s","qty":2}`, variantID)
	_ = postJSON(t, srv.URL+"/api/v1/me/cart/items", token, addBody1, http.StatusCreated)
	addBody2 := fmt.Sprintf(`{"variant_id":"%s","qty":3}`, variantID)
	_ = postJSON(t, srv.URL+"/api/v1/me/cart/items", token, addBody2, http.StatusCreated)
	// GET cart → 1 item, qty=5.
	cart := getJSON(t, srv.URL+"/api/v1/me/cart", token, http.StatusOK)
	cartItems := cart["items"].([]any)
	require.Len(t, cartItems, 1)
	require.EqualValues(t, 5, cartItems[0].(map[string]any)["qty"])

	cartItemID := cartItems[0].(map[string]any)["id"].(string)
	// PATCH qty=10 → OK.
	patchJSON(t, srv.URL+"/api/v1/me/cart/items/"+cartItemID, token, `{"qty":10}`, http.StatusOK)
	// PATCH qty=11 → 400 (binding error: cap at 10).
	patchJSON(t, srv.URL+"/api/v1/me/cart/items/"+cartItemID, token, `{"qty":11}`, http.StatusBadRequest)
	// DELETE item → 204.
	deleteReq(t, srv.URL+"/api/v1/me/cart/items/"+cartItemID, token, http.StatusNoContent)
	// GET → empty.
	cart2 := getJSON(t, srv.URL+"/api/v1/me/cart", token, http.StatusOK)
	require.Len(t, cart2["items"].([]any), 0)

	// === ADDRESS DELETE PROMOTES REMAINING ===
	addr2ID := addr2["id"].(string)
	deleteReq(t, srv.URL+"/api/v1/me/addresses/"+addr2ID, token, http.StatusNoContent)
	listAfter := getJSON(t, srv.URL+"/api/v1/me/addresses", token, http.StatusOK)
	afterItems := listAfter["items"].([]any)
	require.Len(t, afterItems, 1)
	require.True(t, afterItems[0].(map[string]any)["is_default"].(bool))
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func postJSON(t *testing.T, url, token, body string, expectStatus int) map[string]any {
	t.Helper()
	return doJSON(t, "POST", url, token, body, expectStatus)
}

func patchJSON(t *testing.T, url, token, body string, expectStatus int) map[string]any {
	t.Helper()
	return doJSON(t, "PATCH", url, token, body, expectStatus)
}

func getJSON(t *testing.T, url, token string, expectStatus int) map[string]any {
	t.Helper()
	return doJSON(t, "GET", url, token, "", expectStatus)
}

func deleteReq(t *testing.T, url, token string, expectStatus int) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, expectStatus, resp.StatusCode, "url=%s response=%s", url, string(raw))
}

func doJSON(t *testing.T, method, url, token, body string, expectStatus int) map[string]any {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, rdr)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, expectStatus, resp.StatusCode, "url=%s body=%s response=%s", url, body, string(raw))
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
