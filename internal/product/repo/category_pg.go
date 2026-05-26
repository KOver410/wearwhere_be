package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type CategoryPG struct{ db DBTX }

func NewCategoryPG(db DBTX) *CategoryPG { return &CategoryPG{db: db} }

func (r *CategoryPG) List(ctx context.Context) ([]*domain.Category, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, slug, name, display_order FROM categories ORDER BY display_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Category
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (r *CategoryPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	var c domain.Category
	err := r.db.QueryRow(ctx,
		`SELECT id, slug, name, display_order FROM categories WHERE id=$1`, id).
		Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (r *CategoryPG) FindBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	var c domain.Category
	err := r.db.QueryRow(ctx,
		`SELECT id, slug, name, display_order FROM categories WHERE slug=$1`, slug).
		Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}
