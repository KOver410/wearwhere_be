package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type ImagePG struct{ db DBTX }

func NewImagePG(db DBTX) *ImagePG { return &ImagePG{db: db} }

const imageCols = `id, product_id, url, storage_key, alt_text,
                   sort_order, is_primary, created_at`

func scanImage(row pgx.Row) (*domain.Image, error) {
	var i domain.Image
	err := row.Scan(
		&i.ID, &i.ProductID, &i.URL, &i.StorageKey, &i.AltText,
		&i.SortOrder, &i.IsPrimary, &i.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &i, nil
}

func (r *ImagePG) Create(ctx context.Context, productID uuid.UUID, url, storageKey string) (*domain.Image, error) {
	// Compute next sort_order and decide is_primary in one trip.
	var nextOrder int
	var hasAny bool
	if err := r.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) + 1, COUNT(*) > 0
           FROM product_images WHERE product_id=$1`, productID).
		Scan(&nextOrder, &hasAny); err != nil {
		return nil, err
	}
	isPrimary := !hasAny

	row := r.db.QueryRow(ctx,
		`INSERT INTO product_images
           (product_id, url, storage_key, sort_order, is_primary)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING `+imageCols,
		productID, url, storageKey, nextOrder, isPrimary)
	return scanImage(row)
}

func (r *ImagePG) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Image, error) {
	return scanImage(r.db.QueryRow(ctx,
		`SELECT `+imageCols+` FROM product_images
         WHERE id=$1 AND product_id=$2`, id, productID))
}

func (r *ImagePG) ListByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Image, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+imageCols+` FROM product_images
         WHERE product_id=$1 ORDER BY sort_order ASC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Image
	for rows.Next() {
		i, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *ImagePG) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error) {
	// If becoming primary, first demote sibling.
	if req.IsPrimary != nil && *req.IsPrimary {
		if _, err := r.db.Exec(ctx,
			`UPDATE product_images SET is_primary = FALSE
             WHERE product_id=$1 AND id <> $2 AND is_primary`,
			productID, id); err != nil {
			return nil, err
		}
	}
	row := r.db.QueryRow(ctx,
		`UPDATE product_images SET
           sort_order = COALESCE($3, sort_order),
           alt_text   = COALESCE($4, alt_text),
           is_primary = COALESCE($5, is_primary)
         WHERE id=$1 AND product_id=$2
         RETURNING `+imageCols,
		id, productID, req.SortOrder, req.AltText, req.IsPrimary)
	return scanImage(row)
}

func (r *ImagePG) Delete(ctx context.Context, id, productID uuid.UUID) (string, bool, error) {
	var storageKey string
	var wasPrimary bool
	err := r.db.QueryRow(ctx,
		`DELETE FROM product_images WHERE id=$1 AND product_id=$2
         RETURNING storage_key, is_primary`, id, productID).
		Scan(&storageKey, &wasPrimary)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	}
	return storageKey, wasPrimary, err
}

// PromoteNextPrimary sets the lowest sort_order image as primary.
// It is idempotent: if a primary already exists for the product, it is a no-op.
func (r *ImagePG) PromoteNextPrimary(ctx context.Context, productID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE product_images SET is_primary = TRUE
         WHERE id = (
           SELECT id FROM product_images
            WHERE product_id=$1
              AND NOT EXISTS (
                SELECT 1 FROM product_images
                 WHERE product_id=$1 AND is_primary
              )
            ORDER BY sort_order ASC, created_at ASC
            LIMIT 1
         )`, productID)
	return err
}
