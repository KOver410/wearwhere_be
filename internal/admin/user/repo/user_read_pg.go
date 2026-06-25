package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

type UserReadPG struct{ db *pgxpool.Pool }

func NewUserReadPG(db *pgxpool.Pool) *UserReadPG { return &UserReadPG{db: db} }

// orderClauses maps validated (sort, order) enum pairs to fixed ORDER BY
// fragments. Raw user input is never interpolated into SQL.
var orderClauses = map[string]map[string]string{
	domain.SortCreatedAt: {
		domain.OrderAsc:  "created_at ASC",
		domain.OrderDesc: "created_at DESC",
	},
	domain.SortLastLogin: {
		domain.OrderAsc:  "last_login_at ASC NULLS LAST",
		domain.OrderDesc: "last_login_at DESC NULLS LAST",
	},
}

func (r *UserReadPG) ListUsers(ctx context.Context, f domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	orderBy := orderClauses[f.Sort][f.Order]
	if orderBy == "" { // defensive: f is expected pre-normalized
		orderBy = "created_at DESC"
	}

	q := `SELECT id, email, phone, name, role, status,
	             email_verified_at, phone_verified_at, avatar_url, last_login_at, created_at,
	             COUNT(*) OVER() AS total
	      FROM users
	      WHERE deleted_at IS NULL
	        AND ($1 = '' OR email ILIKE '%'||$1||'%'
	                     OR name  ILIKE '%'||$1||'%'
	                     OR phone ILIKE '%'||$1||'%')
	      ORDER BY ` + orderBy + ` -- orderBy is always a hardcoded literal from orderClauses, never user input
	      LIMIT $2 OFFSET $3`

	if f.Page < 1 { // defensive: f is expected pre-normalized
		f.Page = 1
	}
	offset := (f.Page - 1) * f.PageSize
	rows, err := r.db.Query(ctx, q, f.Q, f.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []domain.AdminUserRow
	total := 0
	for rows.Next() {
		var u domain.AdminUserRow
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role, &u.Status,
			&u.EmailVerifiedAt, &u.PhoneVerifiedAt, &u.AvatarURL, &u.LastLoginAt, &u.CreatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, u)
	}
	return items, total, rows.Err()
}
