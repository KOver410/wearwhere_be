package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/shared/hash"
)

type UserPG struct{ db *pgxpool.Pool }

func NewUserPG(db *pgxpool.Pool) *UserPG { return &UserPG{db: db} }

const userColumns = `id, email, phone, password_hash, role, status,
        email_verified_at, phone_verified_at, name, avatar_url, bio,
        last_login_at, created_at, updated_at, deleted_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var role, status string
	err := row.Scan(
		&u.ID, &u.Email, &u.Phone, &u.PasswordHash, &role, &status,
		&u.EmailVerifiedAt, &u.PhoneVerifiedAt, &u.Name, &u.AvatarURL, &u.Bio,
		&u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Role = domain.Role(role)
	u.Status = domain.UserStatus(status)
	return &u, nil
}

func (r *UserPG) Create(ctx context.Context, u *domain.User) error {
	const q = `INSERT INTO users (id, email, phone, password_hash, role, status, name)
	           VALUES ($1, $2, $3, $4, $5, $6, $7)
	           RETURNING created_at, updated_at`
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.Role == "" {
		u.Role = domain.RoleCustomer
	}
	if u.Status == "" {
		u.Status = domain.StatusActive
	}
	return r.db.QueryRow(ctx, q,
		u.ID, u.Email, u.Phone, u.PasswordHash, string(u.Role), string(u.Status), u.Name,
	).Scan(&u.CreatedAt, &u.UpdatedAt)
}

func (r *UserPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 AND deleted_at IS NULL`, id)
	return scanUser(row)
}

func (r *UserPG) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email=$1 AND deleted_at IS NULL`, email)
	return scanUser(row)
}

func (r *UserPG) GetByPhone(ctx context.Context, phone string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE phone=$1 AND deleted_at IS NULL`, phone)
	return scanUser(row)
}

func (r *UserPG) GetBySocial(ctx context.Context, provider domain.OAuthProvider, providerUserID string) (*domain.User, error) {
	const q = `SELECT ` + userColumns + ` FROM users u
	           JOIN social_accounts s ON s.user_id = u.id
	           WHERE s.provider=$1 AND s.provider_user_id=$2 AND u.deleted_at IS NULL`
	row := r.db.QueryRow(ctx, q, string(provider), providerUserID)
	return scanUser(row)
}

func (r *UserPG) UpdateProfile(ctx context.Context, id uuid.UUID, name, avatarURL, bio *string) error {
	const q = `UPDATE users SET
	           name = COALESCE($2, name),
	           avatar_url = COALESCE($3, avatar_url),
	           bio = COALESCE($4, bio),
	           updated_at = NOW()
	           WHERE id=$1 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, id, name, avatarURL, bio)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserPG) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	const q = `UPDATE users SET password_hash=$2, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserPG) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET email_verified_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}

func (r *UserPG) MarkPhoneVerified(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET phone_verified_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}

func (r *UserPG) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET last_login_at=NOW() WHERE id=$1`, id)
	return err
}

func (r *UserPG) SoftDelete(ctx context.Context, id uuid.UUID, emailHash, phoneHash *string, purgeAfter time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE users SET status='deleted', deleted_at=NOW(),
		 email=NULL, phone=NULL, password_hash=NULL, avatar_url=NULL, bio=NULL
		 WHERE id=$1`, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO deleted_accounts (user_id, email_hash, phone_hash, purge_after)
		 VALUES ($1, $2, $3, $4)`,
		id, emailHash, phoneHash, purgeAfter)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserPG) LinkSocial(ctx context.Context, userID uuid.UUID, provider domain.OAuthProvider, providerUserID string, email *string) error {
	const q = `INSERT INTO social_accounts (user_id, provider, provider_user_id, email)
	           VALUES ($1, $2, $3, $4)
	           ON CONFLICT (provider, provider_user_id) DO NOTHING`
	_, err := r.db.Exec(ctx, q, userID, string(provider), providerUserID, email)
	return err
}

func (r *UserPG) PushPasswordHistory(ctx context.Context, userID uuid.UUID, hashStr string) error {
	const q = `INSERT INTO password_history (user_id, password_hash) VALUES ($1, $2)`
	_, err := r.db.Exec(ctx, q, userID, hashStr)
	return err
}

// IsPasswordInHistory checks if `plain` matches any of the last 3 password
// hashes for this user (bcrypt compare done in-memory).
func (r *UserPG) IsPasswordInHistory(ctx context.Context, userID uuid.UUID, plain string) (bool, error) {
	const q = `SELECT password_hash FROM password_history
	           WHERE user_id=$1 ORDER BY created_at DESC LIMIT 3`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return false, err
		}
		if hash.ComparePassword(h, plain) {
			return true, nil
		}
	}
	return false, rows.Err()
}
