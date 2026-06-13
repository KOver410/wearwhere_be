package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
)

type OOTDPg struct{ pool *pgxpool.Pool }

func NewOOTDPg(pool *pgxpool.Pool) *OOTDPg { return &OOTDPg{pool: pool} }

func (r *OOTDPg) CreatePost(ctx context.Context, p *domain.Post, productIDs []uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.QueryRow(ctx,
		`INSERT INTO ootd_posts (id, user_id, caption, photo_urls)
		 VALUES ($1,$2,$3,$4)
		 RETURNING status, like_count, comment_count, created_at, updated_at`,
		p.ID, p.UserID, p.Caption, p.PhotoURLs,
	).Scan(&p.Status, &p.LikeCount, &p.CommentCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return err
	}
	for _, pid := range productIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO ootd_post_products (post_id, product_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, p.ID, pid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// postSelect is the column list + joins for a single/feed post view.
const postSelect = `SELECT p.id, p.user_id, p.caption, p.photo_urls, p.status,
                           p.like_count, p.comment_count, p.created_at, p.updated_at, u.name
                      FROM ootd_posts p JOIN users u ON u.id = p.user_id`

func scanPostView(row pgx.Row) (*domain.PostView, error) {
	var v domain.PostView
	err := row.Scan(&v.ID, &v.UserID, &v.Caption, &v.PhotoURLs, &v.Status,
		&v.LikeCount, &v.CommentCount, &v.CreatedAt, &v.UpdatedAt, &v.AuthorName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *OOTDPg) GetPost(ctx context.Context, id uuid.UUID) (*domain.PostView, error) {
	return scanPostView(r.pool.QueryRow(ctx,
		postSelect+` WHERE p.id=$1 AND p.deleted_at IS NULL AND p.status='published'`, id))
}

func (r *OOTDPg) feedQuery(ctx context.Context, where string, arg any, limit, offset int) ([]*domain.PostView, int, error) {
	var total int
	countSQL := `SELECT COUNT(*) FROM ootd_posts p WHERE ` + where
	var err error
	if arg != nil {
		err = r.pool.QueryRow(ctx, countSQL, arg).Scan(&total)
	} else {
		err = r.pool.QueryRow(ctx, countSQL).Scan(&total)
	}
	if err != nil {
		return nil, 0, err
	}

	q := postSelect + ` WHERE ` + where + ` ORDER BY p.created_at DESC`
	var args []any
	if arg != nil {
		args = append(args, arg)
		q += ` LIMIT $2 OFFSET $3`
		args = append(args, limit, offset)
	} else {
		q += ` LIMIT $1 OFFSET $2`
		args = append(args, limit, offset)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.PostView
	for rows.Next() {
		v, err := scanPostView(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *OOTDPg) FeedList(ctx context.Context, limit, offset int) ([]*domain.PostView, int, error) {
	return r.feedQuery(ctx, `p.deleted_at IS NULL AND p.status='published'`, nil, limit, offset)
}

func (r *OOTDPg) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error) {
	return r.feedQuery(ctx, `p.user_id=$1 AND p.deleted_at IS NULL AND p.status='published'`, userID, limit, offset)
}

func (r *OOTDPg) UpdateCaption(ctx context.Context, postID uuid.UUID, caption *string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE ootd_posts SET caption=$2, updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL`, postID, caption)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *OOTDPg) SoftDeletePost(ctx context.Context, postID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE ootd_posts SET deleted_at=NOW(), updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL`, postID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *OOTDPg) Like(ctx context.Context, postID, userID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx,
		`INSERT INTO ootd_likes (post_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, postID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `UPDATE ootd_posts SET like_count = like_count + 1 WHERE id=$1`, postID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *OOTDPg) Unlike(ctx context.Context, postID, userID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `DELETE FROM ootd_likes WHERE post_id=$1 AND user_id=$2`, postID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `UPDATE ootd_posts SET like_count = like_count - 1 WHERE id=$1`, postID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *OOTDPg) LikedPostIDs(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(postIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT post_id FROM ootd_likes WHERE user_id=$1 AND post_id = ANY($2)`, userID, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *OOTDPg) TagsForPosts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]domain.ProductTag, error) {
	out := map[uuid.UUID][]domain.ProductTag{}
	if len(postIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT jp.post_id, p.id, p.slug, p.name
		   FROM ootd_post_products jp JOIN products p ON p.id = jp.product_id
		  WHERE jp.post_id = ANY($1) AND p.deleted_at IS NULL`, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var postID uuid.UUID
		var t domain.ProductTag
		if err := rows.Scan(&postID, &t.ProductID, &t.Slug, &t.Name); err != nil {
			return nil, err
		}
		out[postID] = append(out[postID], t)
	}
	return out, rows.Err()
}

func (r *OOTDPg) AddComment(ctx context.Context, c *domain.Comment) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	err = tx.QueryRow(ctx,
		`INSERT INTO ootd_comments (post_id, user_id, body) VALUES ($1,$2,$3)
		 RETURNING id, status, created_at`, c.PostID, c.UserID, c.Body).
		Scan(&c.ID, &c.Status, &c.CreatedAt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ootd_posts SET comment_count = comment_count + 1 WHERE id=$1`, c.PostID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *OOTDPg) ListComments(ctx context.Context, postID uuid.UUID, limit, offset int) ([]*domain.CommentView, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ootd_comments WHERE post_id=$1 AND deleted_at IS NULL AND status='published'`, postID).
		Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.post_id, c.user_id, c.body, c.status, c.created_at, u.name
		   FROM ootd_comments c JOIN users u ON u.id = c.user_id
		  WHERE c.post_id=$1 AND c.deleted_at IS NULL AND c.status='published'
		  ORDER BY c.created_at ASC
		  LIMIT $2 OFFSET $3`, postID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.CommentView
	for rows.Next() {
		var v domain.CommentView
		if err := rows.Scan(&v.ID, &v.PostID, &v.UserID, &v.Body, &v.Status, &v.CreatedAt, &v.AuthorName); err != nil {
			return nil, 0, err
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

func (r *OOTDPg) CommentOwner(ctx context.Context, commentID uuid.UUID) (uuid.UUID, error) {
	var owner uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM ootd_comments WHERE id=$1 AND deleted_at IS NULL`, commentID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, err
	}
	return owner, nil
}

func (r *OOTDPg) SoftDeleteComment(ctx context.Context, commentID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var postID uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE ootd_comments SET deleted_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL RETURNING post_id`, commentID).Scan(&postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ootd_posts SET comment_count = comment_count - 1 WHERE id=$1`, postID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
