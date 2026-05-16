// Package repo defines the persistence interfaces used by the auth services.
//
// All repos are kept narrow (no leaky pgx types in signatures) so services can
// be unit-tested against in-memory fakes.
package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
)

var ErrNotFound = errors.New("not found")

type UserRepo interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByPhone(ctx context.Context, phone string) (*domain.User, error)
	GetBySocial(ctx context.Context, provider domain.OAuthProvider, providerUserID string) (*domain.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, name, avatarURL, bio *string) error
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
	MarkPhoneVerified(ctx context.Context, id uuid.UUID) error
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
	SoftDelete(ctx context.Context, id uuid.UUID, emailHash, phoneHash *string, purgeAfter time.Time) error
	LinkSocial(ctx context.Context, userID uuid.UUID, provider domain.OAuthProvider, providerUserID string, email *string) error
	PushPasswordHistory(ctx context.Context, userID uuid.UUID, hash string) error
	IsPasswordInHistory(ctx context.Context, userID uuid.UUID, plain string) (bool, error)
}

type SessionRepo interface {
	Create(ctx context.Context, s *domain.RefreshSession) error
	GetByHash(ctx context.Context, hash string) (*domain.RefreshSession, error)
	Revoke(ctx context.Context, hash string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
	RevokeAllExcept(ctx context.Context, userID uuid.UUID, keepHash string) error
}

// OTPStore lives in Redis: short-lived codes, rate counters.
type OTPStore interface {
	Save(ctx context.Context, key, code string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	IncrCounter(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// AttemptStore handles brute-force counters and lockouts.
type AttemptStore interface {
	IncrFailedLogin(ctx context.Context, contact string, ttl time.Duration) (int64, error)
	ResetFailedLogin(ctx context.Context, contact string) error
	Lock(ctx context.Context, contact string, ttl time.Duration) error
	IsLocked(ctx context.Context, contact string) (bool, error)
}
