package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/hash"
)

type PasswordService struct {
	users    repo.UserRepo
	sessions repo.SessionRepo
	otp      *OTPService
	auth     *AuthService
}

func NewPasswordService(u repo.UserRepo, s repo.SessionRepo, o *OTPService, a *AuthService) *PasswordService {
	return &PasswordService{users: u, sessions: s, otp: o, auth: a}
}

// Forgot triggers an OTP to the user's email or phone (UC06).
// Always returns success-shaped response to avoid leaking which contacts exist.
func (p *PasswordService) Forgot(ctx context.Context, req domain.ForgotPasswordRequest) error {
	if req.Email == "" && req.Phone == "" {
		return domain.ErrEmailOrPhoneRequired
	}

	if req.Email != "" {
		if _, err := p.users.GetByEmail(ctx, req.Email); err == nil {
			return p.otp.Send(ctx, "email", req.Email, PurposeResetPassword)
		}
	} else {
		if _, err := p.users.GetByPhone(ctx, req.Phone); err == nil {
			return p.otp.Send(ctx, "sms", req.Phone, PurposeResetPassword)
		}
	}
	// Pretend success — don't reveal user existence.
	return nil
}

// Reset verifies the OTP and sets the new password (UC06 steps 4-6).
func (p *PasswordService) Reset(ctx context.Context, req domain.ResetPasswordRequest) error {
	if req.Email == "" && req.Phone == "" {
		return domain.ErrEmailOrPhoneRequired
	}
	contact := req.Email
	if contact == "" {
		contact = req.Phone
	}
	if err := p.otp.Verify(ctx, contact, PurposeResetPassword, req.OTP); err != nil {
		return err
	}

	user, err := p.auth.FindUserForVerification(ctx, req.Email, req.Phone)
	if err != nil {
		return err
	}
	return p.updatePassword(ctx, user, req.NewPassword, true)
}

// Change verifies the current password before setting a new one (UC08).
// Per SRS: new password must differ from last 3 passwords; all other sessions
// invalidated.
func (p *PasswordService) Change(ctx context.Context, userID uuid.UUID, req domain.ChangePasswordRequest, keepRefreshHash string) error {
	user, err := p.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrUserNotFound
		}
		return err
	}
	if !user.HasPassword() || !hash.ComparePassword(*user.PasswordHash, req.CurrentPassword) {
		return domain.ErrInvalidCredentials
	}
	if req.CurrentPassword == req.NewPassword {
		return domain.ErrSamePassword
	}
	used, err := p.users.IsPasswordInHistory(ctx, user.ID, req.NewPassword)
	if err != nil {
		return err
	}
	if used {
		return domain.ErrPasswordReuse
	}
	if err := p.updatePassword(ctx, user, req.NewPassword, false); err != nil {
		return err
	}
	if keepRefreshHash != "" {
		return p.sessions.RevokeAllExcept(ctx, user.ID, keepRefreshHash)
	}
	return p.sessions.RevokeAllForUser(ctx, user.ID)
}

func (p *PasswordService) updatePassword(ctx context.Context, user *domain.User, newPass string, revokeAll bool) error {
	hashStr, err := hash.Password(newPass)
	if err != nil {
		return err
	}
	if err := p.users.UpdatePassword(ctx, user.ID, hashStr); err != nil {
		return err
	}
	_ = p.users.PushPasswordHistory(ctx, user.ID, hashStr)
	if revokeAll {
		return p.sessions.RevokeAllForUser(ctx, user.ID)
	}
	return nil
}
