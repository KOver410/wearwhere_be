package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type VariantPG struct{ db DBTX }

func NewVariantPG(db DBTX) *VariantPG { return &VariantPG{db: db} }

const variantCols = `id, product_id, sku, size, color, color_hex, price, stock_qty,
                     is_active, image_id, created_at, updated_at, deleted_at`

func scanVariant(row pgx.Row) (*domain.Variant, error) {
	var v domain.Variant
	err := row.Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.ColorHex,
		&v.Price, &v.StockQty, &v.IsActive, &v.ImageID,
		&v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *VariantPG) Create(ctx context.Context, productID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error) {
	var imageID *uuid.UUID
	if req.ImageID != "" {
		v, err := uuid.Parse(req.ImageID)
		if err != nil {
			return nil, err
		}
		imageID = &v
	}
	var hex *string
	if req.ColorHex != "" {
		hex = &req.ColorHex
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO product_variants
         (product_id, sku, size, color, color_hex, price, stock_qty, image_id)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         RETURNING `+variantCols,
		productID, req.SKU, req.Size, req.Color, hex, req.Price, req.StockQty, imageID)
	v, err := scanVariant(row)
	if err != nil {
		if isUniqueViol(err, "idx_product_variants_size_color") {
			return nil, ErrVariantConflict
		}
		if isUniqueViol(err, "idx_product_variants_sku") {
			return nil, ErrVariantConflict
		}
		return nil, err
	}
	return v, nil
}

func (r *VariantPG) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Variant, error) {
	return scanVariant(r.db.QueryRow(ctx,
		`SELECT `+variantCols+` FROM product_variants
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL`, id, productID))
}

func (r *VariantPG) ListByProduct(ctx context.Context, productID uuid.UUID, onlyActive bool) ([]*domain.Variant, error) {
	q := `SELECT ` + variantCols + ` FROM product_variants
          WHERE product_id=$1 AND deleted_at IS NULL`
	if onlyActive {
		q += ` AND is_active = TRUE`
	}
	q += ` ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, q, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Variant
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *VariantPG) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error) {
	var imageID *uuid.UUID
	if req.ImageID != nil && *req.ImageID != "" {
		v, err := uuid.Parse(*req.ImageID)
		if err != nil {
			return nil, err
		}
		imageID = &v
	}
	row := r.db.QueryRow(ctx,
		`UPDATE product_variants SET
           size       = COALESCE($3, size),
           color      = COALESCE($4, color),
           color_hex  = COALESCE($5, color_hex),
           price      = COALESCE($6, price),
           stock_qty  = COALESCE($7, stock_qty),
           is_active  = COALESCE($8, is_active),
           image_id   = COALESCE($9, image_id),
           updated_at = NOW()
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL
         RETURNING `+variantCols,
		id, productID, req.Size, req.Color, req.ColorHex,
		req.Price, req.StockQty, req.IsActive, imageID)
	v, err := scanVariant(row)
	if err != nil {
		if isUniqueViol(err, "idx_product_variants_size_color") {
			return nil, ErrVariantConflict
		}
		return nil, err
	}
	return v, nil
}

func (r *VariantPG) SoftDelete(ctx context.Context, id, productID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE product_variants SET deleted_at=NOW(), updated_at=NOW()
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL`, id, productID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *VariantPG) FindForPurchase(ctx context.Context, variantID uuid.UUID) (*domain.Variant, *domain.Product, error) {
	var v domain.Variant
	var p domain.Product
	err := r.db.QueryRow(ctx, `
      SELECT
        v.id, v.product_id, v.sku, v.size, v.color, v.color_hex,
        v.price, v.stock_qty, v.is_active, v.image_id,
        v.created_at, v.updated_at, v.deleted_at,
        p.id, p.brand_id, p.category_id, p.slug, p.name, p.description,
        p.status, p.currency, p.sold_count, p.view_count,
        p.created_at, p.updated_at, p.deleted_at
      FROM product_variants v
      JOIN products p ON p.id = v.product_id
      WHERE v.id = $1
        AND v.deleted_at IS NULL AND v.is_active = TRUE
        AND p.deleted_at IS NULL AND p.status = 'active'`,
		variantID,
	).Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.ColorHex,
		&v.Price, &v.StockQty, &v.IsActive, &v.ImageID,
		&v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		&p.ID, &p.BrandID, &p.CategoryID, &p.Slug, &p.Name, &p.Description,
		&p.Status, &p.Currency, &p.SoldCount, &p.ViewCount,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return &v, &p, nil
}
