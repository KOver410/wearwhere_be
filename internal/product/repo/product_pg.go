package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type ProductPG struct{ db DBTX }

func NewProductPG(db DBTX) *ProductPG { return &ProductPG{db: db} }

const productCols = `id, brand_id, category_id, slug, name, description, status,
                     currency, sold_count, view_count, created_at, updated_at, deleted_at`

func scanProduct(row pgx.Row) (*domain.Product, error) {
	var p domain.Product
	var status string
	err := row.Scan(
		&p.ID, &p.BrandID, &p.CategoryID, &p.Slug, &p.Name, &p.Description, &status,
		&p.Currency, &p.SoldCount, &p.ViewCount, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.Status = domain.ProductStatus(status)
	return &p, nil
}

func (r *ProductPG) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) {
	catID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		return nil, err
	}
	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO products (brand_id, category_id, slug, name, description, status)
         VALUES ($1, $2, $3, $4, $5, 'draft')
         RETURNING `+productCols,
		brandID, catID, slug, req.Name, desc)
	p, err := scanProduct(row)
	if err != nil {
		if isUniqueViol(err, "idx_products_brand_slug") {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	return p, nil
}

func (r *ProductPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	return scanProduct(r.db.QueryRow(ctx,
		`SELECT `+productCols+` FROM products WHERE id=$1 AND deleted_at IS NULL`, id))
}

func (r *ProductPG) FindByBrandSlug(ctx context.Context, brandSlug, productSlug string) (*domain.Product, error) {
	return scanProduct(r.db.QueryRow(ctx,
		`SELECT `+productCols+` FROM products p
         JOIN brands b ON b.id = p.brand_id
         WHERE b.slug=$1 AND p.slug=$2 AND p.deleted_at IS NULL AND b.deleted_at IS NULL`,
		brandSlug, productSlug))
}

func (r *ProductPG) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error {
	var catID *uuid.UUID
	if req.CategoryID != nil {
		v, err := uuid.Parse(*req.CategoryID)
		if err != nil {
			return err
		}
		catID = &v
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE products SET
           name        = COALESCE($3, name),
           slug        = COALESCE($4, slug),
           description = COALESCE($5, description),
           category_id = COALESCE($6, category_id),
           status      = COALESCE($7::product_status, status),
           updated_at  = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`,
		id, brandID, req.Name, req.Slug, req.Description, catID, req.Status)
	if err != nil {
		if isUniqueViol(err, "idx_products_brand_slug") {
			return ErrSlugTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProductPG) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE products SET deleted_at=NOW(), updated_at=NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProductPG) ListByBrand(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+productCols+` FROM products
         WHERE brand_id=$1 AND deleted_at IS NULL
         ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		brandID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []*domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM products WHERE brand_id=$1 AND deleted_at IS NULL`,
		brandID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *ProductPG) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(
           SELECT 1 FROM products
           WHERE brand_id=$1 AND slug=$2 AND deleted_at IS NULL)`,
		brandID, slug).Scan(&exists)
	return exists, err
}

func (r *ProductPG) IncrementViewCount(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE products SET view_count = view_count + 1
         WHERE id=$1 AND deleted_at IS NULL`, id)
	return err
}

func (r *ProductPG) SetStyleTags(ctx context.Context, productID uuid.UUID, tagIDs []uuid.UUID) error {
	if _, err := r.db.Exec(ctx,
		`DELETE FROM product_style_tags WHERE product_id=$1`, productID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		if _, err := r.db.Exec(ctx,
			`INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1, $2)
             ON CONFLICT DO NOTHING`, productID, tid); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProductPG) GetStyleTags(ctx context.Context, productID uuid.UUID) ([]*domain.StyleTag, error) {
	rows, err := r.db.Query(ctx,
		`SELECT s.id, s.slug, s.name
           FROM style_tags s
           JOIN product_style_tags pst ON pst.style_tag_id = s.id
          WHERE pst.product_id = $1
          ORDER BY s.name`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.StyleTag
	for rows.Next() {
		var s domain.StyleTag
		if err := rows.Scan(&s.ID, &s.Slug, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func isUniqueViol(err error, indexName string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if indexName == "" {
		return true
	}
	return strings.Contains(pgErr.ConstraintName, indexName) ||
		strings.Contains(pgErr.Message, indexName)
}
