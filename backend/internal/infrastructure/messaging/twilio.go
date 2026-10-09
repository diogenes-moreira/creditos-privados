package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// twilioAPIBase is the Twilio REST API root. Overridden in tests.
const twilioAPIBase = "https://api.twilio.com"

// TwilioConfig holds the credentials and sender identity for WhatsApp delivery.
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	// FromNumber is the WhatsApp-enabled Twilio sender in E.164 (+14155238886).
	FromNumber string
	// TemplateSID is an approved Content template (HX...). WhatsApp requires a
	// template for business-initiated messages, which an OTP always is: the
	// user asks for a code from the app, not from the chat. Without it Twilio
	// only delivers inside an open 24h conversation window (or the sandbox).
	TemplateSID string
	// DefaultDialCode is prepended to stored phones that are not already in
	// E.164, e.g. "54" for Argentina.
	DefaultDialCode string
}

// Enabled reports whether enough is configured to send through Twilio.
func (c TwilioConfig) Enabled() bool {
	return strings.TrimSpace(c.AccountSID) != "" &&
		strings.TrimSpace(c.AuthToken) != "" &&
		strings.TrimSpace(c.FromNumber) != ""
}

// TwilioWhatsAppSender delivers OTP codes as WhatsApp messages through Twilio's
// Messages API. It talks to the REST endpoint directly to avoid pulling in the
// whole Twilio SDK for one call.
type TwilioWhatsAppSender struct {
	cfg     TwilioConfig
	baseURL string
	client  *http.Client
}

// NewTwilioWhatsAppSender builds a Twilio sender, or returns nil when Twilio is
// not configured so the caller can fall back to another channel.
func NewTwilioWhatsAppSender(cfg TwilioConfig) *TwilioWhatsAppSender {
	if !cfg.Enabled() {
		return nil
	}
	return &TwilioWhatsAppSender{
		cfg:     cfg,
		baseURL: twilioAPIBase,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *TwilioWhatsAppSender) Channel() string { return "whatsapp" }

// SendOTP sends the code to the recipient's phone over WhatsApp.
func (s *TwilioWhatsAppSender) SendOTP(ctx context.Context, to port.OTPRecipient, code string) error {
	phone, err := normalizePhone(to.Phone, s.cfg.DefaultDialCode)
	if err != nil {
		return fmt.Errorf("whatsapp OTP: %w", err)
	}

	form := url.Values{}
	form.Set("From", "whatsapp:"+mustNormalize(s.cfg.FromNumber, s.cfg.DefaultDialCode))
	form.Set("To", "whatsapp:"+phone)
	// A whitespace-only template id counts as unset: Secret Manager cannot hold
	// an empty payload, so "no template yet" is stored as a blank placeholder.
	if strings.TrimSpace(s.cfg.TemplateSID) != "" {
		// Approved template with the code as its first variable.
		vars, err := json.Marshal(map[string]string{"1": code})
		if err != nil {
			return fmt.Errorf("whatsapp OTP: encoding template variables: %w", err)
		}
		form.Set("ContentSid", strings.TrimSpace(s.cfg.TemplateSID))
		form.Set("ContentVariables", string(vars))
	} else {
		form.Set("Body", fmt.Sprintf("Tu código de acceso a Prestia es %s. Vence en 10 minutos.", code))
	}

	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", s.baseURL, url.PathEscape(s.cfg.AccountSID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("whatsapp OTP: building request: %w", err)
	}
	req.SetBasicAuth(s.cfg.AccountSID, s.cfg.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp OTP: calling Twilio: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Twilio reports failures as a JSON body with a numeric code; surface it
		// without ever echoing the credentials or the OTP itself.
		var apiErr struct {
			Code     int    `json:"code"`
			Message  string `json:"message"`
			MoreInfo string `json:"more_info"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message != "" {
			return fmt.Errorf("whatsapp OTP: twilio %d (code %d): %s", resp.StatusCode, apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("whatsapp OTP: twilio returned %s", resp.Status)
	}
	return nil
}

// normalizePhone turns a stored phone into the E.164 form Twilio expects.
// Formatting characters are dropped; a number without a country code gets the
// deployment country's dial code.
func normalizePhone(raw, defaultDialCode string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("recipient has no phone number on file")
	}

	plus := strings.HasPrefix(trimmed, "+")
	var digits strings.Builder
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	number := digits.String()
	if number == "" {
		return "", fmt.Errorf("phone number %q has no digits", raw)
	}

	if !plus {
		dial := strings.TrimPrefix(strings.TrimSpace(defaultDialCode), "+")
		if dial != "" && !strings.HasPrefix(number, dial) {
			number = dial + number
		}
	}

	// E.164 allows at most 15 digits, and a country code plus subscriber number
	// is never shorter than 8 in the markets this runs in.
	if len(number) < 8 || len(number) > 15 {
		return "", fmt.Errorf("phone number %q is not a valid E.164 number", raw)
	}
	return "+" + number, nil
}

// mustNormalize is used for the configured sender number, which is operator
// input rather than user data: if it cannot be normalized it is passed through
// so Twilio reports the misconfiguration in its own error message.
func mustNormalize(raw, defaultDialCode string) string {
	if n, err := normalizePhone(raw, defaultDialCode); err == nil {
		return n
	}
	return strings.TrimSpace(raw)
}
