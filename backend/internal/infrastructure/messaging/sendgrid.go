package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// sendGridAPIBase is SendGrid's v3 mail endpoint root. Overridden in tests.
const sendGridAPIBase = "https://api.sendgrid.com"

// SendGridConfig configures one-time codes delivered by email.
//
// Email is the sensible channel for staff: an administrator reads mail at a
// desk, while a rancher in a vereda reads WhatsApp. Nothing forces both to use
// the same channel.
type SendGridConfig struct {
	APIKey string
	// FromEmail must be a verified sender identity in SendGrid, otherwise the
	// API rejects the send with 403.
	FromEmail string
	FromName  string
}

// Enabled reports whether enough is configured to send through SendGrid.
func (c SendGridConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.FromEmail) != ""
}

// SendGridOTPSender delivers one-time codes over email.
type SendGridOTPSender struct {
	cfg     SendGridConfig
	baseURL string
	client  *http.Client
}

// NewSendGridOTPSender builds a SendGrid sender, or returns nil when SendGrid
// is not configured so the caller can fall back to another channel.
func NewSendGridOTPSender(cfg SendGridConfig) *SendGridOTPSender {
	if !cfg.Enabled() {
		return nil
	}
	return &SendGridOTPSender{
		cfg:     cfg,
		baseURL: sendGridAPIBase,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *SendGridOTPSender) Channel() string { return "email" }

// SendOTP emails the code to the recipient.
func (s *SendGridOTPSender) SendOTP(ctx context.Context, to port.OTPRecipient, code string) error {
	address := strings.TrimSpace(to.Email)
	if address == "" {
		return fmt.Errorf("email OTP: recipient has no email address on file")
	}

	from := map[string]any{"email": strings.TrimSpace(s.cfg.FromEmail)}
	if name := strings.TrimSpace(s.cfg.FromName); name != "" {
		from["name"] = name
	}

	text := fmt.Sprintf(
		"Tu código de acceso a CuppoGan es %s.\n\n"+
			"Vence en 10 minutos y se usa una sola vez.\n"+
			"Si no pediste este código, ignorá este mensaje.",
		code)

	payload := map[string]any{
		"personalizations": []any{
			map[string]any{"to": []any{map[string]any{"email": address}}},
		},
		"from":    from,
		"subject": fmt.Sprintf("%s es tu código de acceso a CuppoGan", code),
		"content": []any{
			map[string]any{"type": "text/plain", "value": text},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("email OTP: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v3/mail/send", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email OTP: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email OTP: calling SendGrid: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// SendGrid accepts a send with 202 and no body.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr struct {
			Errors []struct {
				Message string `json:"message"`
				Field   string `json:"field"`
			} `json:"errors"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if len(apiErr.Errors) > 0 {
			// Surface SendGrid's reason without ever echoing the key or the code.
			msgs := make([]string, 0, len(apiErr.Errors))
			for _, e := range apiErr.Errors {
				if e.Field != "" {
					msgs = append(msgs, fmt.Sprintf("%s (%s)", e.Message, e.Field))
					continue
				}
				msgs = append(msgs, e.Message)
			}
			return fmt.Errorf("email OTP: sendgrid %d: %s", resp.StatusCode, strings.Join(msgs, "; "))
		}
		return fmt.Errorf("email OTP: sendgrid returned %s", resp.Status)
	}
	return nil
}
