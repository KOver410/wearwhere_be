package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

// DBTX is the subset both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type BrandPG struct{ db DBTX }

func NewBrandPG(db DBTX) *BrandPG { return &BrandPG{db: db} }

const brandCols = `id, slug, name, owner_user_id, story, logo_url, banner_url,
                   website_url, status, shipping_flat_fee_vnd, verified_at, created_at, updated_at, deleted_at`

func scanBrand(row pgx.Row) (*domain.Brand, error) {
	var b domain.Brand
	var status string
	err := row.Scan(
		&b.ID, &b.Slug, &b.Name, &b.OwnerUserID, &b.Story, &b.LogoURL,
		&b.BannerURL, &b.WebsiteURL, &status, &b.ShippingFlatFeeVND, &b.VerifiedAt,
		&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	b.Status = domain.BrandStatus(status)
	return &b, nil
}

func (r *BrandPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	return scanBrand(r.db.QueryRow(ctx,
		`SELECT `+brandCols+` FROM brands WHERE id=$1 AND deleted_at IS NULL`, id))
}

func (r *BrandPG) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
	return scanBrand(r.db.QueryRow(ctx,
		`SELECT `+brandCols+` FROM brands WHERE slug=$1 AND deleted_at IS NULL`, slug))
}

func (r *BrandPG) FindByOwnerUserID(ctx context.Context, userID uuid.UUID) (*domain.Brand, error) {
	return scanBrand(r.db.QueryRow(ctx,
		`SELECT `+brandCols+` FROM brands
         WHERE owner_user_id=$1 AND deleted_at IS NULL`, userID))
}

func (r *BrandPG) Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
	const q = `UPDATE brands SET
        name        = COALESCE($2, name),
        slug        = COALESCE($3, slug),
        story       = COALESCE($4, story),
        logo_url    = COALESCE($5, logo_url),
        banner_url  = COALESCE($6, banner_url),
        website_url = COALESCE($7, website_url),
        updated_at  = NOW()
        WHERE id=$1 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, id,
		req.Name, req.Slug, req.Story, req.LogoURL, req.BannerURL, req.WebsiteURL)
	if err != nil {
		if isUniqueViolation(err, "brands_slug_key") {
			return ErrSlugTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *BrandPG) List(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
	// SELECT path: $1=limit, $2=offset, ($3=q optional)
	selectArgs := []any{limit, offset}
	selectWhere := "deleted_at IS NULL AND status = 'active'"
	if q != "" {
		selectArgs = append(selectArgs, q)
		selectWhere += " AND name % $3"
	}

	orderBy := "created_at DESC"
	switch sort {
	case "a-z":
		orderBy = "name ASC"
	case "newest":
		orderBy = "created_at DESC"
	}

	selectSQL := `SELECT ` + brandCols + ` FROM brands WHERE ` + selectWhere +
		` ORDER BY ` + orderBy + ` LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []*domain.Brand
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// COUNT path: independent parameter numbering. ($1=q optional)
	countArgs := []any{}
	countWhere := "deleted_at IS NULL AND status = 'active'"
	if q != "" {
		countArgs = append(countArgs, q)
		countWhere += " AND name % $1"
	}
	countSQL := `SELECT COUNT(*) FROM brands WHERE ` + countWhere

	var total int
	if err := r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// isUniqueViolation detects Postgres unique_violation (23505) optionally
// constrained to a named index.
func isUniqueViolation(err error, indexName string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if indexName == "" {
		return true
	}
	return strings.Contains(pgErr.ConstraintName, indexName) ||
		strings.Contains(pgErr.Message, indexName)
}
