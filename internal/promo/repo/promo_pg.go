// internal/promo/repo/promo_pg.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
)

type PromoPG struct{ pool *pgxpool.Pool }

func NewPromoPG(pool *pgxpool.Pool) *PromoPG { return &PromoPG{pool: pool} }

const promoCols = `id, code, description, discount_type, discount_value,
                   max_discount_vnd, min_order_value_vnd, starts_at, ends_at,
                   is_active, created_at, updated_at`

func scanPromo(row pgx.Row) (*domain.PromoCode, error) {
	var p domain.PromoCode
	var desc *string
	err := row.Scan(
		&p.ID, &p.Code, &desc, &p.DiscountType, &p.DiscountValue,
		&p.MaxDiscountVND, &p.MinOrderValueVND, &p.StartsAt, &p.EndsAt,
		&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if desc != nil {
		p.Description = *desc
	}
	return &p, nil
}

// db returns db if non-nil, else the pool (so reads work outside a tx).
func (r *PromoPG) db(db DBTX) DBTX {
	if db == nil {
		return r.pool
	}
	return db
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *PromoPG) GetActiveByCode(ctx context.Context, db DBTX, code string) (*domain.PromoCode, error) {
	return scanPromo(r.db(db).QueryRow(ctx,
		`SELECT `+promoCols+` FROM promo_codes WHERE code = $1 AND is_active = TRUE`, code))
}

func (r *PromoPG) GetActiveByCodeForUpdate(ctx context.Context, db DBTX, code string) (*domain.PromoCode, error) {
	return scanPromo(r.db(db).QueryRow(ctx,
		`SELECT `+promoCols+` FROM promo_codes WHERE code = $1 AND is_active = TRUE FOR UPDATE`, code))
}

func (r *PromoPG) HasRedeemed(ctx context.Context, db DBTX, promoID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db(db).QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM promo_redemptions WHERE promo_code_id = $1 AND user_id = $2)`,
		promoID, userID).Scan(&exists)
	return exists, err
}

func (r *PromoPG) InsertRedemption(ctx context.Context, db DBTX, promoID, userID, orderID uuid.UUID, discountVND int64) error {
	_, err := r.db(db).Exec(ctx,
		`INSERT INTO promo_redemptions (promo_code_id, user_id, order_id, discount_vnd)
		 VALUES ($1, $2, $3, $4)`,
		promoID, userID, orderID, discountVND)
	if err != nil && isUniqueViolation(err) {
		return ErrAlreadyRedeemed
	}
	return err
}

func (r *PromoPG) Create(ctx context.Context, p *domain.PromoCode) error {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO promo_codes
		   (code, description, discount_type, discount_value, max_discount_vnd,
		    min_order_value_vnd, starts_at, ends_at, is_active)
		 VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at, updated_at`,
		p.Code, p.Description, p.DiscountType, p.DiscountValue, p.MaxDiscountVND,
		p.MinOrderValueVND, p.StartsAt, p.EndsAt, p.IsActive)
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrCodeConflict
	}
	return err
}

func (r *PromoPG) Update(ctx context.Context, p *domain.PromoCode) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE promo_codes
		    SET description = NULLIF($2,''),
		        max_discount_vnd = $3,
		        min_order_value_vnd = $4,
		        starts_at = $5,
		        ends_at = $6,
		        is_active = $7,
		        updated_at = NOW()
		  WHERE id = $1`,
		p.ID, p.Description, p.MaxDiscountVND, p.MinOrderValueVND,
		p.StartsAt, p.EndsAt, p.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PromoPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.PromoCode, error) {
	return scanPromo(r.pool.QueryRow(ctx,
		`SELECT `+promoCols+` FROM promo_codes WHERE id = $1`, id))
}

func (r *PromoPG) List(ctx context.Context, page, pageSize int, activeOnly bool) ([]*domain.PromoCode, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	where := ""
	if activeOnly {
		where = ` WHERE is_active = TRUE`
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM promo_codes`+where).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+promoCols+` FROM promo_codes`+where+`
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.PromoCode
	for rows.Next() {
		p, err := scanPromo(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}
