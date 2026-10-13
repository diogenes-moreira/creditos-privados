package messaging

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSendGridConfig() SendGridConfig {
	return SendGridConfig{
		APIKey:    "SG.secret-key",
		FromEmail: "hola@cuppogan.com",
		FromName:  "CuppoGan",
	}
}

func newStubbedSendGrid(t *testing.T, cfg SendGridConfig, status int, body string) (*SendGridOTPSender, *map[string]any, *string) {
	t.Helper()
	captured := &map[string]any{}
	authHeader := new(string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, captured))
		*authHeader = r.Header.Get("Authorization")
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v3/mail/send", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	sender := NewSendGridOTPSender(cfg)
	require.NotNil(t, sender)
	sender.baseURL = srv.URL
	return sender, captured, authHeader
}

func TestSendGridConfigEnabled(t *testing.T) {
	assert.True(t, testSendGridConfig().Enabled())

	for _, tc := range []struct {
		name   string
		mutate func(*SendGridConfig)
	}{
		{"no api key", func(c *SendGridConfig) { c.APIKey = "" }},
		{"no sender address", func(c *SendGridConfig) { c.FromEmail = "" }},
		{"blank api key", func(c *SendGridConfig) { c.APIKey = "   " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testSendGridConfig()
			tc.mutate(&cfg)
			assert.False(t, cfg.Enabled())
			assert.Nil(t, NewSendGridOTPSender(cfg))
		})
	}
}

func TestSendGridSendOTP(t *testing.T) {
	// SendGrid accepts a send with 202 and an empty body.
	sender, body, auth := newStubbedSendGrid(t, testSendGridConfig(), http.StatusAccepted, "")

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com", Phone: "+573000010100"}, "123456")
	require.NoError(t, err)

	personalizations := (*body)["personalizations"].([]any)
	require.Len(t, personalizations, 1)
	to := personalizations[0].(map[string]any)["to"].([]any)
	assert.Equal(t, "ana@example.com", to[0].(map[string]any)["email"])

	from := (*body)["from"].(map[string]any)
	assert.Equal(t, "hola@cuppogan.com", from["email"])
	assert.Equal(t, "CuppoGan", from["name"])

	// The code appears in both the subject and the body, so it is visible in a
	// notification preview without opening the mail.
	assert.Contains(t, (*body)["subject"], "123456")
	content := (*body)["content"].([]any)
	assert.Contains(t, content[0].(map[string]any)["value"], "123456")

	assert.Equal(t, "Bearer SG.secret-key", *auth)
}

func TestSendGridOmitsEmptyFromName(t *testing.T) {
	cfg := testSendGridConfig()
	cfg.FromName = ""
	sender, body, _ := newStubbedSendGrid(t, cfg, http.StatusAccepted, "")

	require.NoError(t, sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com"}, "123456"))

	from := (*body)["from"].(map[string]any)
	assert.Equal(t, "hola@cuppogan.com", from["email"])
	assert.Nil(t, from["name"], "an empty name must be omitted, not sent blank")
}

func TestSendGridSurfacesAPIError(t *testing.T) {
	// 403 with this message is what an unverified sender identity returns.
	sender, _, _ := newStubbedSendGrid(t, testSendGridConfig(), http.StatusForbidden,
		`{"errors":[{"message":"The from address does not match a verified Sender Identity","field":"from"}]}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verified Sender Identity")
	assert.Contains(t, err.Error(), "from")
	// Neither the code nor the API key may leak into the error.
	assert.NotContains(t, err.Error(), "123456")
	assert.NotContains(t, err.Error(), "SG.secret-key")
}

func TestSendGridRejectsMissingEmail(t *testing.T) {
	sender := NewSendGridOTPSender(testSendGridConfig())
	require.NotNil(t, sender)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+573000010100"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no email address")
}

func TestSendGridChannel(t *testing.T) {
	assert.Equal(t, "email", NewSendGridOTPSender(testSendGridConfig()).Channel())
}

func TestNewOTPSenderSelectsSendGrid(t *testing.T) {

	t.Run("explicit sendgrid wins over whatsapp", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{
			Provider: "sendgrid", SendGrid: testSendGridConfig(), Twilio: testTwilioConfig()})
		assert.Equal(t, "email", s.Channel())
	})

	t.Run("whatsapp is preferred when nothing is specified", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{SendGrid: testSendGridConfig(), Twilio: testTwilioConfig()})
		assert.Equal(t, "whatsapp", s.Channel())
	})

	t.Run("falls back to email when no gateway is configured", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{SendGrid: testSendGridConfig()})
		assert.Equal(t, "email", s.Channel())
	})

	t.Run("named provider that is unconfigured still delivers", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{Provider: "sendgrid", Twilio: testTwilioConfig()})
		assert.Equal(t, "whatsapp", s.Channel(), "must not silently send nothing")
	})
}

func TestSendGridSubjectLeadsWithTheCode(t *testing.T) {
	sender, body, _ := newStubbedSendGrid(t, testSendGridConfig(), http.StatusAccepted, "")

	require.NoError(t, sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com"}, "987654"))

	// Mail clients truncate subjects, so the code goes first.
	assert.True(t, strings.HasPrefix((*body)["subject"].(string), "987654"),
		"subject should start with the code, got %q", (*body)["subject"])
}
