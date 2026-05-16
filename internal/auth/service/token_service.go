package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/hash"
	jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
)

type TokenService struct {
	issuer     *jwtsvc.Issuer
	sessions   repo.SessionRepo
	refreshTTL time.Duration
}

func NewTokenService(issuer *jwtsvc.Issuer, sessions repo.SessionRepo, refreshTTL time.Duration) *TokenService {
	return &TokenService{issuer: issuer, sessions: sessions, refreshTTL: refreshTTL}
}

// Issue creates both access and refresh tokens and persists the refresh hash.
func (s *TokenService) Issue(ctx context.Context, u *domain.User, ua, ip string) (domain.AuthTokens, error) {
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	access, exp, err := s.issuer.IssueAccess(u.ID.String(), string(u.Role), email)
	if err != nil {
		return domain.AuthTokens{}, err
	}

	refresh, err := generateRefreshToken()
	if err != nil {
		return domain.AuthTokens{}, err
	}
	rhash := hash.SHA256Hex(refresh)

	expiresAt := time.Now().Add(s.refreshTTL)
	if err := s.sessions.Create(ctx, &domain.RefreshSession{
		UserID:    u.ID,
		TokenHash: rhash,
		UserAgent: ua,
		IP:        ip,
		ExpiresAt: expiresAt,
	}); err != nil {
		return domain.AuthTokens{}, err
	}

	return domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresAt:    exp,
	}, nil
}

// Refresh rotates the refresh token: validates the old hash, revokes it,
// returns a brand-new access+refresh pair.
func (s *TokenService) Refresh(ctx context.Context, refresh string, userLookup func(context.Context, uuid.UUID) (*domain.User, error), ua, ip string) (domain.AuthTokens, error) {
	rhash := hash.SHA256Hex(refresh)
	sess, err := s.sessions.GetByHash(ctx, rhash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.AuthTokens{}, domain.ErrInvalidRefresh
		}
		return domain.AuthTokens{}, err
	}
	if sess.RevokedAt != nil || time.Now().After(sess.ExpiresAt) {
		return domain.AuthTokens{}, domain.ErrInvalidRefresh
	}

	user, err := userLookup(ctx, sess.UserID)
	if err != nil {
		return domain.AuthTokens{}, domain.ErrInvalidRefresh
	}
	if !user.IsActive() {
		return domain.AuthTokens{}, domain.ErrAccountDeleted
	}

	// Rotate: revoke old session before minting new
	if err := s.sessions.Revoke(ctx, rhash); err != nil {
		return domain.AuthTokens{}, err
	}
	return s.Issue(ctx, user, ua, ip)
}

func (s *TokenService) Revoke(ctx context.Context, refresh string) error {
	return s.sessions.Revoke(ctx, hash.SHA256Hex(refresh))
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
