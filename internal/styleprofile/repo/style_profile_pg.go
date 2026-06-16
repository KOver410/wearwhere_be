package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

type StyleProfilePG struct{ db DBTX }

func NewStyleProfilePG(db DBTX) *StyleProfilePG { return &StyleProfilePG{db: db} }

func (r *StyleProfilePG) UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT x FROM unnest($1::uuid[]) AS x
		 WHERE NOT EXISTS (SELECT 1 FROM style_tags st WHERE st.id = x)`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Upsert runs the profile write + tag replacement as a single multi-statement
// query. Postgres executes every data-modifying CTE exactly once and to
// completion, so the delete+insert of tags is atomic even on a connection pool.
func (r *StyleProfilePG) Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	_, err := r.db.Exec(ctx,
		`WITH up AS (
		    INSERT INTO style_profiles (user_id, budget_min, budget_max, onboarded_at)
		    VALUES ($1, $2, $3, NOW())
		    ON CONFLICT (user_id) DO UPDATE
		      SET budget_min = EXCLUDED.budget_min,
		          budget_max = EXCLUDED.budget_max,
		          updated_at = NOW()
		    RETURNING user_id
		 ),
		 del AS (
		    DELETE FROM style_profile_tags WHERE user_id = $1
		 )
		 INSERT INTO style_profile_tags (user_id, style_tag_id)
		 SELECT $1, t FROM unnest($4::uuid[]) AS t
		 ON CONFLICT DO NOTHING`,
		p.UserID, p.BudgetMin, p.BudgetMax, p.StyleTagIDs)
	if err != nil {
		return nil, err
	}
	return r.Load(ctx, p.UserID)
}

func (r *StyleProfilePG) Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v := &domain.StyleProfileView{UserID: userID}
	err := r.db.QueryRow(ctx,
		`SELECT budget_min, budget_max, onboarded_at
		   FROM style_profiles WHERE user_id = $1`, userID).
		Scan(&v.BudgetMin, &v.BudgetMax, &v.OnboardedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT st.id, st.slug, st.name
		   FROM style_profile_tags spt
		   JOIN style_tags st ON st.id = spt.style_tag_id
		  WHERE spt.user_id = $1
		  ORDER BY st.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref domain.StyleTagRef
		var id uuid.UUID
		if err := rows.Scan(&id, &ref.Slug, &ref.Name); err != nil {
			return nil, err
		}
		ref.ID = id.String()
		v.StyleTags = append(v.StyleTags, ref)
	}
	return v, rows.Err()
}
