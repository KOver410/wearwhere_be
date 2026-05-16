package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
)

type SessionPG struct{ db *pgxpool.Pool }

func NewSessionPG(db *pgxpool.Pool) *SessionPG { return &SessionPG{db: db} }

func (r *SessionPG) Create(ctx context.Context, s *domain.RefreshSession) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	const q = `INSERT INTO refresh_sessions (id, user_id, refresh_token_hash, user_agent, ip, expires_at)
	           VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, $6)`
	_, err := r.db.Exec(ctx, q, s.ID, s.UserID, s.TokenHash, s.UserAgent, s.IP, s.ExpiresAt)
	return err
}

func (r *SessionPG) GetByHash(ctx context.Context, hashStr string) (*domain.RefreshSession, error) {
	const q = `SELECT id, user_id, refresh_token_hash, COALESCE(user_agent,''),
	           COALESCE(host(ip),''), expires_at, revoked_at, created_at
	           FROM refresh_sessions WHERE refresh_token_hash=$1`
	row := r.db.QueryRow(ctx, q, hashStr)
	var s domain.RefreshSession
	err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.UserAgent, &s.IP,
		&s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *SessionPG) Revoke(ctx context.Context, hashStr string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at=NOW()
		 WHERE refresh_token_hash=$1 AND revoked_at IS NULL`, hashStr)
	return err
}

func (r *SessionPG) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at=NOW()
		 WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

func (r *SessionPG) RevokeAllExcept(ctx context.Context, userID uuid.UUID, keepHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at=NOW()
		 WHERE user_id=$1 AND revoked_at IS NULL AND refresh_token_hash<>$2`,
		userID, keepHash)
	return err
}
