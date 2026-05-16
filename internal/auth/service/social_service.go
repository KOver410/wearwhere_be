package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/config"
	"github.com/wearwhere/wearwhere_be/internal/shared/apple"
)

// SocialService verifies provider tokens server-side, then upserts a local
// user. We never trust client-sent profile data — only verified provider
// responses.
type SocialService struct {
	users    repo.UserRepo
	tokens   *TokenService
	cfg      config.OAuthConfig
	http     *http.Client
	appleVer *apple.Verifier
}

func NewSocialService(u repo.UserRepo, t *TokenService, cfg config.OAuthConfig) *SocialService {
	s := &SocialService{
		users: u, tokens: t, cfg: cfg,
		http: &http.Client{Timeout: 5 * time.Second},
	}
	if len(cfg.AppleClientIDs) > 0 {
		s.appleVer = apple.NewVerifier(cfg.AppleClientIDs...)
	}
	return s
}

type socialProfile struct {
	ProviderUserID string
	Email          string // may be empty (Apple after first login, FB limited)
	Name           string
}

// ── Google ID token ──
// Verified via the tokeninfo endpoint. For production at scale prefer JWKs
// caching, but tokeninfo is sufficient for the project's volume and keeps
// the code dependency-light.
func (s *SocialService) verifyGoogle(ctx context.Context, idToken string) (*socialProfile, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, domain.ErrSocialTokenInvalid
	}
	var payload struct {
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !s.cfg.AllowsGoogleAud(payload.Aud) {
		return nil, domain.ErrSocialTokenInvalid
	}
	return &socialProfile{ProviderUserID: payload.Sub, Email: payload.Email, Name: payload.Name}, nil
}

// ── Apple identity token ──
// Verified via internal/shared/apple: fetches Apple's JWKs (cached 15min),
// validates RS256 signature, iss=https://appleid.apple.com, aud=AppleClientID,
// and exp. Apple only returns email/name on the first login; subsequent
// logins are matched by `sub`.
func (s *SocialService) verifyApple(ctx context.Context, identityToken string) (*socialProfile, error) {
	if s.appleVer == nil {
		return nil, domain.ErrSocialTokenInvalid
	}
	claims, err := s.appleVer.Verify(ctx, identityToken)
	if err != nil {
		return nil, domain.ErrSocialTokenInvalid
	}
	return &socialProfile{
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		// Apple does not include `name` in the ID token — client must pass it
		// separately on first sign-in. We leave it empty here.
	}, nil
}

// LoginOrRegister handles the full flow: verify provider token → look up by
// (provider, providerUserID) → fall back to email match → create new user.
func (s *SocialService) LoginOrRegister(ctx context.Context, provider domain.OAuthProvider, providerToken, ua, ip string) (*domain.AuthResponse, error) {
	if !provider.Valid() {
		return nil, domain.ErrSocialTokenInvalid
	}

	var (
		profile *socialProfile
		err     error
	)
	switch provider {
	case domain.ProviderGoogle:
		profile, err = s.verifyGoogle(ctx, providerToken)
	case domain.ProviderApple:
		profile, err = s.verifyApple(ctx, providerToken)
	}
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetBySocial(ctx, provider, profile.ProviderUserID)
	if err == nil {
		return s.issueFor(ctx, user, ua, ip)
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}

	// Try to link to an existing account by email
	if profile.Email != "" {
		if existing, err := s.users.GetByEmail(ctx, profile.Email); err == nil {
			emailPtr := &profile.Email
			if err := s.users.LinkSocial(ctx, existing.ID, provider, profile.ProviderUserID, emailPtr); err != nil {
				return nil, err
			}
			return s.issueFor(ctx, existing, ua, ip)
		}
	}

	// New user
	newUser := &domain.User{
		ID:     uuid.New(),
		Name:   profile.Name,
		Role:   domain.RoleCustomer,
		Status: domain.StatusActive,
	}
	if profile.Email != "" {
		newUser.Email = &profile.Email
		now := time.Now()
		newUser.EmailVerifiedAt = &now // provider already verified
	}
	if err := s.users.Create(ctx, newUser); err != nil {
		return nil, err
	}
	emailPtr := (*string)(nil)
	if profile.Email != "" {
		emailPtr = &profile.Email
	}
	if err := s.users.LinkSocial(ctx, newUser.ID, provider, profile.ProviderUserID, emailPtr); err != nil {
		return nil, err
	}
	return s.issueFor(ctx, newUser, ua, ip)
}

func (s *SocialService) issueFor(ctx context.Context, u *domain.User, ua, ip string) (*domain.AuthResponse, error) {
	if !u.IsActive() {
		return nil, domain.ErrAccountDeleted
	}
	tokens, err := s.tokens.Issue(ctx, u, ua, ip)
	if err != nil {
		return nil, err
	}
	return &domain.AuthResponse{User: domain.ToUserResponse(u), Tokens: tokens}, nil
}
