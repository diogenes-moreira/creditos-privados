package messaging

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKapsoConfig() KapsoConfig {
	return KapsoConfig{
		APIKey:          "kapso-secret-key",
		PhoneNumberID:   "110987654321",
		DefaultDialCode: "57",
	}
}

// newStubbedKapso points a sender at a local test server and captures the
// decoded request body and the API key header.
func newStubbedKapso(t *testing.T, cfg KapsoConfig, status int, body string) (*KapsoWhatsAppSender, *map[string]any, *string) {
	t.Helper()
	captured := &map[string]any{}
	apiKey := new(string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, captured))
		*apiKey = r.Header.Get("X-API-Key")
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/110987654321/messages", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	sender := NewKapsoWhatsAppSender(cfg)
	require.NotNil(t, sender)
	sender.baseURL = srv.URL
	return sender, captured, apiKey
}

func TestKapsoConfigEnabled(t *testing.T) {
	assert.True(t, testKapsoConfig().Enabled())

	for _, tc := range []struct {
		name   string
		mutate func(*KapsoConfig)
	}{
		{"no api key", func(c *KapsoConfig) { c.APIKey = "" }},
		{"no phone number id", func(c *KapsoConfig) { c.PhoneNumberID = "" }},
		{"blank api key", func(c *KapsoConfig) { c.APIKey = "   " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testKapsoConfig()
			tc.mutate(&cfg)
			assert.False(t, cfg.Enabled())
			assert.Nil(t, NewKapsoWhatsAppSender(cfg))
		})
	}
}

func TestKapsoSendOTPTextBody(t *testing.T) {
	sender, body, key := newStubbedKapso(t, testKapsoConfig(), http.StatusOK,
		`{"messages":[{"id":"wamid.ABC"}]}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com", Phone: "+573000010100"}, "123456")
	require.NoError(t, err)

	assert.Equal(t, "whatsapp", (*body)["messaging_product"])
	// Meta addresses recipients by digits, with no leading plus.
	assert.Equal(t, "573000010100", (*body)["to"])
	assert.Equal(t, "text", (*body)["type"])
	text := (*body)["text"].(map[string]any)
	assert.Contains(t, text["body"], "123456")
	assert.Nil(t, (*body)["template"])
	assert.Equal(t, "kapso-secret-key", *key)
}

func TestKapsoSendOTPTemplate(t *testing.T) {
	cfg := testKapsoConfig()
	cfg.TemplateName = "codigo_acceso"
	cfg.TemplateLanguage = "es_CO"
	sender, body, _ := newStubbedKapso(t, cfg, http.StatusOK, `{"messages":[{"id":"wamid.ABC"}]}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+573000010100"}, "987654")
	require.NoError(t, err)

	assert.Equal(t, "template", (*body)["type"])
	tmpl := (*body)["template"].(map[string]any)
	assert.Equal(t, "codigo_acceso", tmpl["name"])
	assert.Equal(t, "es_CO", tmpl["language"].(map[string]any)["code"])

	// The code travels as the first positional body parameter, filling {{1}}.
	components := tmpl["components"].([]any)
	require.Len(t, components, 1)
	params := components[0].(map[string]any)["parameters"].([]any)
	require.Len(t, params, 1)
	assert.Equal(t, "987654", params[0].(map[string]any)["text"])
	// A positional parameter must not carry a name.
	assert.Nil(t, params[0].(map[string]any)["parameter_name"])
	assert.Nil(t, (*body)["text"])
}

func TestKapsoTemplateDefaultsLanguage(t *testing.T) {
	cfg := testKapsoConfig()
	cfg.TemplateName = "codigo_acceso"
	sender, body, _ := newStubbedKapso(t, cfg, http.StatusOK, `{}`)

	require.NoError(t, sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+573000010100"}, "111222"))

	tmpl := (*body)["template"].(map[string]any)
	assert.Equal(t, "es", tmpl["language"].(map[string]any)["code"])
}

func TestKapsoSurfacesAPIError(t *testing.T) {
	// 131047 is Meta's "re-engagement required": the 24h window has closed.
	sender, _, _ := newStubbedKapso(t, testKapsoConfig(), http.StatusUnprocessableEntity,
		`{"error":{"message":"Message failed to send because more than 24 hours have passed","type":"OAuthException","code":131047}}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+573000010100"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "131047")
	assert.Contains(t, err.Error(), "24 hours")
	// Neither the code nor the API key may leak into the error.
	assert.NotContains(t, err.Error(), "123456")
	assert.NotContains(t, err.Error(), "kapso-secret-key")
}

func TestKapsoRejectsMissingPhone(t *testing.T) {
	sender := NewKapsoWhatsAppSender(testKapsoConfig())
	require.NotNil(t, sender)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no phone number")
}

func TestKapsoAppliesDefaultDialCode(t *testing.T) {
	sender, body, _ := newStubbedKapso(t, testKapsoConfig(), http.StatusOK, `{}`)

	// Stored without a country code: the deployment country's dial code applies.
	require.NoError(t, sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "3000010100"}, "123456"))

	assert.Equal(t, "573000010100", (*body)["to"])
}

func TestKapsoChannel(t *testing.T) {
	assert.Equal(t, "whatsapp", NewKapsoWhatsAppSender(testKapsoConfig()).Channel())
}

func TestNewOTPSenderProviderSelection(t *testing.T) {

	t.Run("explicit kapso", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{
			Provider: "kapso", Kapso: testKapsoConfig(), Twilio: testTwilioConfig()})
		assert.IsType(t, &KapsoWhatsAppSender{}, s)
	})

	t.Run("explicit twilio", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{
			Provider: "twilio", Kapso: testKapsoConfig(), Twilio: testTwilioConfig()})
		assert.IsType(t, &TwilioWhatsAppSender{}, s)
	})

	t.Run("prefers kapso when unspecified", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{Kapso: testKapsoConfig(), Twilio: testTwilioConfig()})
		assert.IsType(t, &KapsoWhatsAppSender{}, s)
	})

	t.Run("falls through when the chosen one is unconfigured", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{Provider: "kapso", Twilio: testTwilioConfig()})
		assert.IsType(t, &TwilioWhatsAppSender{}, s, "must not silently send nothing")
	})

	t.Run("console when nothing is configured", func(t *testing.T) {
		s := NewOTPSender(OTPProviders{})
		assert.Equal(t, "console", s.Channel())
	})
}
