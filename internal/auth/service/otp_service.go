package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/config"
	"github.com/wearwhere/wearwhere_be/internal/shared/mailer"
	"github.com/wearwhere/wearwhere_be/internal/shared/sms"
)

const (
	PurposeVerifyEmail   = "verify_email"
	PurposeVerifyPhone   = "verify_phone"
	PurposeResetPassword = "reset_password"
)

type OTPService struct {
	store  repo.OTPStore
	mailer mailer.Mailer
	sms    sms.Sender
	limit  config.LimitConfig
}

func NewOTPService(store repo.OTPStore, m mailer.Mailer, s sms.Sender, limit config.LimitConfig) *OTPService {
	return &OTPService{store: store, mailer: m, sms: s, limit: limit}
}

// Send generates a 6-digit OTP, stores it in Redis, and dispatches it
// via email or SMS. Enforces a per-contact hourly limit per SRS UC06.
func (s *OTPService) Send(ctx context.Context, channel, contact, purpose string) error {
	if !validPurpose(purpose) {
		return domain.ErrInvalidOTP
	}

	counterKey := fmt.Sprintf("otp:count:%s:%s", purpose, contact)
	n, err := s.store.IncrCounter(ctx, counterKey, time.Hour)
	if err != nil {
		return err
	}
	if n > int64(s.limit.OTPMaxPerHour) {
		return domain.ErrTooManyOTPRequests
	}

	code, err := genOTP()
	if err != nil {
		return err
	}
	ttl := time.Duration(s.limit.OTPTTLMinutes) * time.Minute
	if err := s.store.Save(ctx, otpKey(purpose, contact), code, ttl); err != nil {
		return err
	}

	subject := otpSubject(purpose)
	body := otpEmailBody(code, ttl)

	switch channel {
	case "email":
		return s.mailer.Send(ctx, contact, subject, body)
	case "sms":
		return s.sms.Send(ctx, contact, fmt.Sprintf("WearWhere: your code is %s (valid %dm)", code, s.limit.OTPTTLMinutes))
	default:
		return errors.New("invalid channel")
	}
}

// Verify returns nil if the OTP matches; deletes the code on success.
func (s *OTPService) Verify(ctx context.Context, contact, purpose, otp string) error {
	if !validPurpose(purpose) {
		return domain.ErrInvalidOTP
	}
	key := otpKey(purpose, contact)
	stored, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrInvalidOTP
		}
		return err
	}
	if stored != otp {
		return domain.ErrInvalidOTP
	}
	_ = s.store.Delete(ctx, key)
	return nil
}

func otpKey(purpose, contact string) string { return fmt.Sprintf("otp:%s:%s", purpose, contact) }

func validPurpose(p string) bool {
	switch p {
	case PurposeVerifyEmail, PurposeVerifyPhone, PurposeResetPassword:
		return true
	}
	return false
}

func otpSubject(purpose string) string {
	switch purpose {
	case PurposeVerifyEmail:
		return "[WearWhere] Verify your email"
	case PurposeResetPassword:
		return "[WearWhere] Reset your password"
	}
	return "[WearWhere] Your verification code"
}

func otpEmailBody(code string, ttl time.Duration) string {
	return fmt.Sprintf(`<p>Your WearWhere verification code is:</p>
<h2 style="letter-spacing:4px">%s</h2>
<p>This code is valid for %d minutes. If you didn't request this, ignore this email.</p>`,
		code, int(ttl.Minutes()))
}

func genOTP() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
