package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/config"
	"github.com/wearwhere/wearwhere_be/internal/shared/hash"
)

type AuthService struct {
	users    repo.UserRepo
	attempts repo.AttemptStore
	tokens   *TokenService
	otp      *OTPService
	limit    config.LimitConfig
}

func NewAuthService(u repo.UserRepo, a repo.AttemptStore, t *TokenService, o *OTPService, limit config.LimitConfig) *AuthService {
	return &AuthService{users: u, attempts: a, tokens: t, otp: o, limit: limit}
}

// Register creates a new customer account (UC03). Either email or phone is
// required. After creation, an OTP is sent to verify the contact channel.
func (s *AuthService) Register(ctx context.Context, req domain.RegisterRequest, ua, ip string) (*domain.AuthResponse, error) {
	if req.Email == "" && req.Phone == "" {
		return nil, domain.ErrEmailOrPhoneRequired
	}

	if req.Email != "" {
		if _, err := s.users.GetByEmail(ctx, req.Email); err == nil {
			return nil, domain.ErrEmailExists
		} else if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}
	if req.Phone != "" {
		if _, err := s.users.GetByPhone(ctx, req.Phone); err == nil {
			return nil, domain.ErrPhoneExists
		} else if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}

	hashStr, err := hash.Password(req.Password)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:           uuid.New(),
		Name:         req.Name,
		PasswordHash: &hashStr,
		Role:         domain.RoleCustomer,
		Status:       domain.StatusActive,
	}
	if req.Email != "" {
		user.Email = &req.Email
	}
	if req.Phone != "" {
		user.Phone = &req.Phone
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	_ = s.users.PushPasswordHistory(ctx, user.ID, hashStr)

	// Fire-and-forget: send verification OTP. Failure here shouldn't block
	// the registration; the client can call /otp/send to retry.
	if req.Email != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.otp.Send(ctx, "email", req.Email, PurposeVerifyEmail)
		}()
	} else if req.Phone != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.otp.Send(ctx, "sms", req.Phone, PurposeVerifyPhone)
		}()
	}

	tokens, err := s.tokens.Issue(ctx, user, ua, ip)
	if err != nil {
		return nil, err
	}
	return &domain.AuthResponse{User: domain.ToUserResponse(user), Tokens: tokens}, nil
}

// Login authenticates by email/phone + password (UC04). Enforces brute-force
// protection: 5 failed attempts within 15 minutes = 15-min lockout.
func (s *AuthService) Login(ctx context.Context, req domain.LoginRequest, ua, ip string) (*domain.AuthResponse, error) {
	contact, user, err := s.findByContact(ctx, req.Email, req.Phone)
	if err != nil {
		return nil, err
	}

	locked, err := s.attempts.IsLocked(ctx, contact)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, domain.ErrAccountLocked
	}

	if !user.HasPassword() || !hash.ComparePassword(*user.PasswordHash, req.Password) {
		n, err := s.attempts.IncrFailedLogin(ctx, contact, time.Duration(s.limit.LoginLockoutMinutes)*time.Minute)
		if err == nil && n >= int64(s.limit.LoginMaxAttempts) {
			_ = s.attempts.Lock(ctx, contact, time.Duration(s.limit.LoginLockoutMinutes)*time.Minute)
			_ = s.attempts.ResetFailedLogin(ctx, contact)
			return nil, domain.ErrAccountLocked
		}
		return nil, domain.ErrInvalidCredentials
	}

	_ = s.attempts.ResetFailedLogin(ctx, contact)
	_ = s.users.TouchLastLogin(ctx, user.ID)

	tokens, err := s.tokens.Issue(ctx, user, ua, ip)
	if err != nil {
		return nil, err
	}
	return &domain.AuthResponse{User: domain.ToUserResponse(user), Tokens: tokens}, nil
}

// LoginAs is the role-gated variant used by the Brand Portal (UC41) and
// Admin CMS (UC52). It performs the same credential check as Login but
// rejects any user whose role does not match `expected`. Customers cannot
// access the brand/admin portals through this path.
func (s *AuthService) LoginAs(ctx context.Context, expected domain.Role, req domain.LoginRequest, ua, ip string) (*domain.AuthResponse, error) {
	resp, err := s.Login(ctx, req, ua, ip)
	if err != nil {
		return nil, err
	}
	if domain.Role(resp.User.Role) != expected {
		// Surface as invalid creds, not "forbidden", to avoid leaking which
		// emails belong to brand/admin accounts.
		return nil, domain.ErrInvalidCredentials
	}
	return resp, nil
}

// Refresh rotates tokens (handler passes the refresh token).
func (s *AuthService) Refresh(ctx context.Context, refresh, ua, ip string) (domain.AuthTokens, error) {
	return s.tokens.Refresh(ctx, refresh, s.users.GetByID, ua, ip)
}

// Logout revokes only the supplied refresh token (UC05).
func (s *AuthService) Logout(ctx context.Context, refresh string) error {
	return s.tokens.Revoke(ctx, refresh)
}

func (s *AuthService) findByContact(ctx context.Context, email, phone string) (string, *domain.User, error) {
	switch {
	case email != "":
		u, err := s.users.GetByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return "", nil, domain.ErrInvalidCredentials
			}
			return "", nil, err
		}
		return email, u, nil
	case phone != "":
		u, err := s.users.GetByPhone(ctx, phone)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return "", nil, domain.ErrInvalidCredentials
			}
			return "", nil, err
		}
		return phone, u, nil
	}
	return "", nil, domain.ErrEmailOrPhoneRequired
}

// FindUserForVerification used by OTP verify endpoints to mark email/phone verified.
func (s *AuthService) FindUserForVerification(ctx context.Context, email, phone string) (*domain.User, error) {
	_, u, err := s.findByContact(ctx, email, phone)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return nil, domain.ErrUserNotFound
	}
	return u, err
}

func (s *AuthService) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	return s.users.MarkEmailVerified(ctx, id)
}

func (s *AuthService) MarkPhoneVerified(ctx context.Context, id uuid.UUID) error {
	return s.users.MarkPhoneVerified(ctx, id)
}
