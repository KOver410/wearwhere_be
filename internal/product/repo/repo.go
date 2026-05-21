// Package repo defines persistence interfaces for the product module.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

var (
	ErrNotFound        = errors.New("product: not found")
	ErrSlugTaken       = errors.New("product: slug taken")
	ErrVariantConflict = errors.New("product: variant conflict")
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

type ProductRepo interface {
	Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	FindByBrandSlug(ctx context.Context, brandSlug, productSlug string) (*domain.Product, error)
	Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error
	SoftDelete(ctx context.Context, id, brandID uuid.UUID) error
	ListByBrand(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error)
	SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error)
	IncrementViewCount(ctx context.Context, id uuid.UUID) error
	SetStyleTags(ctx context.Context, productID uuid.UUID, tagIDs []uuid.UUID) error
	GetStyleTags(ctx context.Context, productID uuid.UUID) ([]*domain.StyleTag, error)
}

type VariantRepo interface {
	Create(ctx context.Context, productID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error)
	FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Variant, error)
	ListByProduct(ctx context.Context, productID uuid.UUID, onlyActive bool) ([]*domain.Variant, error)
	Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error)
	SoftDelete(ctx context.Context, id, productID uuid.UUID) error
	// FindForPurchase returns the variant and its parent product only if both are
	// active, in-stock-capable, and not soft-deleted. Returns ErrNotFound if the
	// variant is missing, inactive, soft-deleted, or its product is not active.
	FindForPurchase(ctx context.Context, variantID uuid.UUID) (*domain.Variant, *domain.Product, error)
}

type ImageRepo interface {
	Create(ctx context.Context, productID uuid.UUID, url, storageKey string) (*domain.Image, error)
	FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Image, error)
	ListByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Image, error)
	Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error)
	Delete(ctx context.Context, id, productID uuid.UUID) (storageKey string, wasPrimary bool, err error)
	PromoteNextPrimary(ctx context.Context, productID uuid.UUID) error
}

type CategoryRepo interface {
	List(ctx context.Context) ([]*domain.Category, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Category, error)
	FindBySlug(ctx context.Context, slug string) (*domain.Category, error)
}

type StyleTagRepo interface {
	List(ctx context.Context) ([]*domain.StyleTag, error)
	FindBySlugs(ctx context.Context, slugs []string) ([]*domain.StyleTag, error)
}

type CatalogRepo interface {
	List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error)
	Detail(ctx context.Context, brandSlug, productSlug string) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)
	DetailByID(ctx context.Context, id uuid.UUID) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)
	Suggestions(ctx context.Context, q string, limit int) ([]string, error)
}
