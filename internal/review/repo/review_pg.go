package repo

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
)

type ReviewPG struct{ pool *pgxpool.Pool }

func NewReviewPG(pool *pgxpool.Pool) *ReviewPG { return &ReviewPG{pool: pool} }

// querier is the subset shared by *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func recompute(ctx context.Context, q querier, productID uuid.UUID) error {
	_, err := q.Exec(ctx,
		`UPDATE products p SET
		   avg_rating = COALESCE((SELECT AVG(rating) FROM product_reviews
		                          WHERE product_id = p.id AND deleted_at IS NULL AND status='published'), 0),
		   review_count = (SELECT COUNT(*) FROM product_reviews
		                   WHERE product_id = p.id AND deleted_at IS NULL AND status='published')
		 WHERE p.id = $1`, productID)
	return err
}

func (r *ReviewPG) ProductExists(ctx context.Context, productID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND deleted_at IS NULL)`, productID).Scan(&ok)
	return ok, err
}

func (r *ReviewPG) HasDeliveredPurchase(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM order_items oi
		   JOIN sub_orders so ON so.id = oi.sub_order_id
		   JOIN orders o      ON o.id  = so.order_id
		   WHERE oi.product_id = $1 AND o.user_id = $2 AND so.status = 'delivered')`,
		productID, userID).Scan(&ok)
	return ok, err
}

func (r *ReviewPG) Create(ctx context.Context, rv *domain.Review) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.QueryRow(ctx,
		`INSERT INTO product_reviews (product_id, user_id, rating, body, fit)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, status, created_at, updated_at`,
		rv.ProductID, rv.UserID, rv.Rating, rv.Body, rv.Fit,
	).Scan(&rv.ID, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicate
		}
		return err
	}
	if err := recompute(ctx, tx, rv.ProductID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	var rv domain.Review
	err := r.pool.QueryRow(ctx,
		`SELECT id, product_id, user_id, rating, body, fit, status, created_at, updated_at
		   FROM product_reviews WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&rv.ID, &rv.ProductID, &rv.UserID, &rv.Rating, &rv.Body, &rv.Fit, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rv, nil
}

func (r *ReviewPG) Update(ctx context.Context, id uuid.UUID, rating int, body string, fit *string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var productID uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE product_reviews SET rating=$2, body=$3, fit=$4, updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL
		 RETURNING product_id`, id, rating, body, fit).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := recompute(ctx, tx, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var productID uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE product_reviews SET deleted_at=NOW(), updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL
		 RETURNING product_id`, id).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := recompute(ctx, tx, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) ListByProduct(ctx context.Context, productID uuid.UUID, f ListFilter) ([]*domain.ReviewView, int, error) {
	where := "r.product_id = $1 AND r.deleted_at IS NULL AND r.status='published'"
	args := []any{productID}
	if f.Rating != 0 {
		args = append(args, f.Rating)
		where += " AND r.rating = $" + strconv.Itoa(len(args))
	}
	if f.Fit != "" {
		args = append(args, f.Fit)
		where += " AND r.fit = $" + strconv.Itoa(len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM product_reviews r WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "r.created_at DESC"
	switch f.Sort {
	case "rating_high":
		orderBy = "r.rating DESC, r.created_at DESC"
	case "rating_low":
		orderBy = "r.rating ASC, r.created_at DESC"
	}

	args = append(args, f.Limit, f.Offset)
	q := `SELECT r.id, r.product_id, r.user_id, r.rating, r.body, r.fit, r.status,
	             r.created_at, r.updated_at, u.name
	        FROM product_reviews r
	        JOIN users u ON u.id = r.user_id
	       WHERE ` + where + `
	       ORDER BY ` + orderBy + `
	       LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.ReviewView
	for rows.Next() {
		var v domain.ReviewView
		if err := rows.Scan(&v.ID, &v.ProductID, &v.UserID, &v.Rating, &v.Body, &v.Fit,
			&v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ReviewerName); err != nil {
			return nil, 0, err
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

func (r *ReviewPG) Aggregate(ctx context.Context, productID uuid.UUID) (Aggregate, error) {
	var a Aggregate
	err := r.pool.QueryRow(ctx,
		`SELECT avg_rating, review_count FROM products WHERE id=$1`, productID).
		Scan(&a.AvgRating, &a.ReviewCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Aggregate{}, ErrNotFound
		}
		return Aggregate{}, err
	}
	return a, nil
}
