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

// kapsoAPIBase is Kapso's Meta-compatible WhatsApp proxy. Overridden in tests.
const kapsoAPIBase = "https://api.kapso.ai/meta/whatsapp/v24.0"

// KapsoConfig holds the credentials and sender identity for WhatsApp delivery
// through Kapso, which fronts the WhatsApp Cloud API with Meta's own request
// shape.
type KapsoConfig struct {
	// APIKey is a project API key from the Kapso dashboard.
	APIKey string
	// PhoneNumberID is the connected WhatsApp number's id, not the number
	// itself: it is the first path segment of the send endpoint.
	PhoneNumberID string
	// TemplateName is an approved template whose first body parameter is the
	// code. WhatsApp only allows free-form text inside an open 24h customer
	// service window, and an OTP is always business-initiated, so production
	// needs this. Without it the sender falls back to a text body, which works
	// in testing while a window is open.
	TemplateName string
	// TemplateLanguage is the template's language code, e.g. "es" or "es_CO".
	TemplateLanguage string
	// DefaultDialCode is prepended to stored phones that are not already E.164.
	DefaultDialCode string
}

// Enabled reports whether enough is configured to send through Kapso.
func (c KapsoConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.PhoneNumberID) != ""
}

// KapsoWhatsAppSender delivers OTP codes as WhatsApp messages through Kapso.
type KapsoWhatsAppSender struct {
	cfg     KapsoConfig
	baseURL string
	client  *http.Client
}

// NewKapsoWhatsAppSender builds a Kapso sender, or returns nil when Kapso is
// not configured so the caller can fall back to another channel.
func NewKapsoWhatsAppSender(cfg KapsoConfig) *KapsoWhatsAppSender {
	if !cfg.Enabled() {
		return nil
	}
	return &KapsoWhatsAppSender{
		cfg:     cfg,
		baseURL: kapsoAPIBase,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *KapsoWhatsAppSender) Channel() string { return "whatsapp" }

// SendOTP sends the code to the recipient's phone over WhatsApp.
func (s *KapsoWhatsAppSender) SendOTP(ctx context.Context, to port.OTPRecipient, code string) error {
	phone, err := normalizePhone(to.Phone, s.cfg.DefaultDialCode)
	if err != nil {
		return fmt.Errorf("whatsapp OTP: %w", err)
	}
	// Meta addresses recipients by digits, without the leading plus.
	recipient := strings.TrimPrefix(phone, "+")

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                recipient,
	}
	if name := strings.TrimSpace(s.cfg.TemplateName); name != "" {
		language := strings.TrimSpace(s.cfg.TemplateLanguage)
		if language == "" {
			language = "es"
		}
		payload["type"] = "template"
		payload["template"] = map[string]any{
			"name":     name,
			"language": map[string]any{"code": language},
			// Positional parameter: fills {{1}} with the code.
			"components": []any{
				map[string]any{
					"type": "body",
					"parameters": []any{
						map[string]any{"type": "text", "text": code},
					},
				},
			},
		}
	} else {
		payload["recipient_type"] = "individual"
		payload["type"] = "text"
		payload["text"] = map[string]any{
			"body": fmt.Sprintf("Tu código de acceso a CuppoGan es %s. Vence en 10 minutos.", code),
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp OTP: encoding request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/messages", s.baseURL, strings.TrimSpace(s.cfg.PhoneNumberID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("whatsapp OTP: building request: %w", err)
	}
	req.Header.Set("X-API-Key", s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp OTP: calling Kapso: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Kapso returns Meta's error envelope. Surface the message and code
		// without ever echoing the API key or the OTP itself.
		var apiErr struct {
			Error struct {
				Message      string `json:"message"`
				Code         int    `json:"code"`
				ErrorSubcode int    `json:"error_subcode"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error.Message != "" {
			return fmt.Errorf("whatsapp OTP: kapso %d (code %d): %s",
				resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message)
		}
		return fmt.Errorf("whatsapp OTP: kapso returned %s", resp.Status)
	}
	return nil
}
