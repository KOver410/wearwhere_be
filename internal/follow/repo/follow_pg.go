package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
)

type FollowPG struct{ pool *pgxpool.Pool }

func NewFollowPG(pool *pgxpool.Pool) *FollowPG { return &FollowPG{pool: pool} }

func (r *FollowPG) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, id).Scan(&ok)
	return ok, err
}

func (r *FollowPG) BrandExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1 AND deleted_at IS NULL)`, id).Scan(&ok)
	return ok, err
}

// toggleCounter runs an insert/delete + conditional counter update + reads the new count, all in one tx.
func (r *FollowPG) toggleCounter(ctx context.Context, writeSQL string, writeArgs []any, counterSQL string, countSQL string, countArg uuid.UUID) (int, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, writeSQL, writeArgs...)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, counterSQL, countArg); err != nil {
			return 0, err
		}
	}
	var count int
	if err := tx.QueryRow(ctx, countSQL, countArg).Scan(&count); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *FollowPG) FollowUser(ctx context.Context, follower, followee uuid.UUID) (int, error) {
	return r.toggleCounter(ctx,
		`INSERT INTO user_follows (follower_id, followee_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		[]any{follower, followee},
		`UPDATE users SET follower_count = follower_count + 1 WHERE id=$1`,
		`SELECT follower_count FROM users WHERE id=$1`, followee)
}

func (r *FollowPG) UnfollowUser(ctx context.Context, follower, followee uuid.UUID) (int, error) {
	return r.toggleCounter(ctx,
		`DELETE FROM user_follows WHERE follower_id=$1 AND followee_id=$2`,
		[]any{follower, followee},
		`UPDATE users SET follower_count = follower_count - 1 WHERE id=$1`,
		`SELECT follower_count FROM users WHERE id=$1`, followee)
}

func (r *FollowPG) FollowBrand(ctx context.Context, userID, brandID uuid.UUID) (int, error) {
	return r.toggleCounter(ctx,
		`INSERT INTO brand_follows (user_id, brand_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		[]any{userID, brandID},
		`UPDATE brands SET follower_count = follower_count + 1 WHERE id=$1`,
		`SELECT follower_count FROM brands WHERE id=$1`, brandID)
}

func (r *FollowPG) UnfollowBrand(ctx context.Context, userID, brandID uuid.UUID) (int, error) {
	return r.toggleCounter(ctx,
		`DELETE FROM brand_follows WHERE user_id=$1 AND brand_id=$2`,
		[]any{userID, brandID},
		`UPDATE brands SET follower_count = follower_count - 1 WHERE id=$1`,
		`SELECT follower_count FROM brands WHERE id=$1`, brandID)
}

func (r *FollowPG) ListFollowingUsers(ctx context.Context, follower uuid.UUID, limit, offset int) ([]domain.FollowingUserItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_follows WHERE follower_id=$1`, follower).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.name, u.avatar_url, u.follower_count
		   FROM user_follows f JOIN users u ON u.id = f.followee_id
		  WHERE f.follower_id=$1 AND u.deleted_at IS NULL
		  ORDER BY f.created_at DESC LIMIT $2 OFFSET $3`, follower, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.FollowingUserItem
	for rows.Next() {
		var it domain.FollowingUserItem
		var id uuid.UUID
		if err := rows.Scan(&id, &it.Name, &it.AvatarURL, &it.FollowerCount); err != nil {
			return nil, 0, err
		}
		it.ID = id.String()
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (r *FollowPG) ListFollowingBrands(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.FollowingBrandItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM brand_follows WHERE user_id=$1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT b.id, b.slug, b.name, b.logo_url, b.follower_count
		   FROM brand_follows f JOIN brands b ON b.id = f.brand_id
		  WHERE f.user_id=$1 AND b.deleted_at IS NULL
		  ORDER BY f.created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.FollowingBrandItem
	for rows.Next() {
		var it domain.FollowingBrandItem
		var id uuid.UUID
		if err := rows.Scan(&id, &it.Slug, &it.Name, &it.LogoURL, &it.FollowerCount); err != nil {
			return nil, 0, err
		}
		it.ID = id.String()
		out = append(out, it)
	}
	return out, total, rows.Err()
}
