package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type BlockPG struct{ pool *pgxpool.Pool }

func NewBlockPG(pool *pgxpool.Pool) *BlockPG { return &BlockPG{pool: pool} }

func (r *BlockPG) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, id).Scan(&ok)
	return ok, err
}

func (r *BlockPG) Block(ctx context.Context, blocker, blocked uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		blocker, blocked)
	return err
}

func (r *BlockPG) Unblock(ctx context.Context, blocker, blocked uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`, blocker, blocked)
	return err
}

func (r *BlockPG) ListBlocked(ctx context.Context, blocker uuid.UUID, limit, offset int) ([]domain.BlockedUserItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_blocks b JOIN users u ON u.id = b.blocked_id
		  WHERE b.blocker_id=$1 AND u.deleted_at IS NULL`, blocker).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.name, u.avatar_url
		   FROM user_blocks b JOIN users u ON u.id = b.blocked_id
		  WHERE b.blocker_id=$1 AND u.deleted_at IS NULL
		  ORDER BY b.created_at DESC LIMIT $2 OFFSET $3`, blocker, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.BlockedUserItem
	for rows.Next() {
		var it domain.BlockedUserItem
		var id uuid.UUID
		if err := rows.Scan(&id, &it.Name, &it.AvatarURL); err != nil {
			return nil, 0, err
		}
		it.ID = id.String()
		out = append(out, it)
	}
	return out, total, rows.Err()
}
