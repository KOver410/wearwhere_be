package mailer

import (
	"context"
	"fmt"
	"log"

	"github.com/wearwhere/wearwhere_be/internal/config"
	gomail "gopkg.in/gomail.v2"
)

type SMTPMailer struct {
	cfg config.SMTPConfig
}

func NewSMTP(cfg config.SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	if m.cfg.Host == "" {
		// dev fallback: print to stdout so the OTP / reset link can still be tested
		log.Printf("[mailer:stub] to=%s subject=%s\n---\n%s\n---", to, subject, htmlBody)
		return nil
	}

	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", m.cfg.FromEmail, m.cfg.FromName)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", htmlBody)

	dialer := gomail.NewDialer(m.cfg.Host, m.cfg.Port, m.cfg.Username, m.cfg.Password)

	done := make(chan error, 1)
	go func() { done <- dialer.DialAndSend(msg) }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("smtp send: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
