package sms

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/wearwhere/wearwhere_be/internal/config"
)

type TwilioSender struct {
	cfg    config.SMSConfig
	client *http.Client
}

func NewTwilio(cfg config.SMSConfig) *TwilioSender {
	return &TwilioSender{cfg: cfg, client: http.DefaultClient}
}

func (s *TwilioSender) Send(ctx context.Context, to, body string) error {
	if s.cfg.AccountSID == "" {
		log.Printf("[sms:stub] to=%s body=%s", to, body)
		return nil
	}

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.cfg.AccountSID)
	form := url.Values{}
	form.Set("From", s.cfg.FromNumber)
	form.Set("To", to)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.cfg.AccountSID, s.cfg.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio error %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
