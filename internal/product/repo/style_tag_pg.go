package repo

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type StyleTagPG struct{ db DBTX }

func NewStyleTagPG(db DBTX) *StyleTagPG { return &StyleTagPG{db: db} }

func (r *StyleTagPG) List(ctx context.Context) ([]*domain.StyleTag, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, slug, name FROM style_tags ORDER BY name`)
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

func (r *StyleTagPG) FindBySlugs(ctx context.Context, slugs []string) ([]*domain.StyleTag, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, slug, name FROM style_tags WHERE slug = ANY($1)`, slugs)
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
