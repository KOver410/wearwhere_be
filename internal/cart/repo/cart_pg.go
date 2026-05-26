package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/cart/domain"
)

type CartPG struct{ db DBTX }

func NewCartPG(db DBTX) *CartPG { return &CartPG{db: db} }

const itemCols = `id, user_id, variant_id, qty, price_snapshot,
                  currency_snapshot, added_at, updated_at`

func scanItem(row pgx.Row) (*domain.CartItem, error) {
	var i domain.CartItem
	err := row.Scan(
		&i.ID, &i.UserID, &i.VariantID, &i.Qty, &i.PriceSnapshot,
		&i.CurrencySnapshot, &i.AddedAt, &i.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &i, nil
}

func (r *CartPG) UpsertAdd(ctx context.Context, userID, variantID uuid.UUID, qty int, price float64) (*domain.CartItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
         VALUES ($1, $2, $3, $4, 'VND')
         ON CONFLICT (user_id, variant_id) DO UPDATE
           SET qty            = LEAST(cart_items.qty + EXCLUDED.qty, 10),
               price_snapshot = EXCLUDED.price_snapshot,
               updated_at     = NOW()
         RETURNING `+itemCols,
		userID, variantID, qty, price))
}

func (r *CartPG) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CartItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`SELECT `+itemCols+` FROM cart_items WHERE id=$1 AND user_id=$2`, id, userID))
}

func (r *CartPG) FindByVariant(ctx context.Context, userID, variantID uuid.UUID) (*domain.CartItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`SELECT `+itemCols+` FROM cart_items WHERE user_id=$1 AND variant_id=$2`,
		userID, variantID))
}

func (r *CartPG) UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int, price float64) (*domain.CartItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`UPDATE cart_items
            SET qty=$3, price_snapshot=$4, updated_at=NOW()
          WHERE id=$1 AND user_id=$2
        RETURNING `+itemCols,
		id, userID, qty, price))
}

func (r *CartPG) Delete(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM cart_items WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *CartPG) Clear(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM cart_items WHERE user_id=$1`, userID)
	return err
}

// ListView joins variants/products/brands so soft-deleted variants still appear
// (with Unavailable=true) until the user explicitly removes them.
func (r *CartPG) ListView(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error) {
	rows, err := r.db.Query(ctx, `
      SELECT
        ci.id, ci.qty, ci.price_snapshot, ci.currency_snapshot, ci.added_at,
        v.id, v.sku, v.size, v.color, v.color_hex, v.stock_qty, v.price, v.is_active, v.deleted_at,
        p.id, p.slug, p.name, p.status, p.deleted_at,
        (SELECT url FROM product_images
           WHERE product_id = p.id AND is_primary
           ORDER BY sort_order ASC LIMIT 1) AS primary_image_url,
        b.id, b.slug, b.name
      FROM cart_items ci
      JOIN product_variants v ON v.id = ci.variant_id
      JOIN products p ON p.id = v.product_id
      JOIN brands b ON b.id = p.brand_id
     WHERE ci.user_id = $1
     ORDER BY ci.added_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.CartItemView
	for rows.Next() {
		v := &domain.CartItemView{}
		var vIsActive bool
		var vDeletedAt, pDeletedAt *time.Time
		var pStatus string
		if err := rows.Scan(
			&v.ID, &v.Qty, &v.PriceSnapshot, &v.CurrencySnapshot, &v.AddedAt,
			&v.VariantID, &v.SKU, &v.Size, &v.Color, &v.ColorHex, &v.StockQty,
			&v.CurrentPrice, &vIsActive, &vDeletedAt,
			&v.ProductID, &v.ProductSlug, &v.ProductName, &pStatus, &pDeletedAt,
			&v.PrimaryImageURL,
			&v.BrandID, &v.BrandSlug, &v.BrandName,
		); err != nil {
			return nil, err
		}
		// Availability precedence: variant_deleted > variant_inactive > product_unavailable.
		switch {
		case vDeletedAt != nil:
			v.Unavailable = true
			reason := "variant_deleted"
			v.UnavailableReason = &reason
		case !vIsActive:
			v.Unavailable = true
			reason := "variant_inactive"
			v.UnavailableReason = &reason
		case pDeletedAt != nil || pStatus != "active":
			v.Unavailable = true
			reason := "product_unavailable"
			v.UnavailableReason = &reason
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
