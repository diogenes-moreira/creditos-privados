package messaging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTwilioConfig() TwilioConfig {
	return TwilioConfig{
		AccountSID:      "AC123",
		AuthToken:       "secret-token",
		FromNumber:      "+14155238886",
		DefaultDialCode: "54",
	}
}

// newStubbedSender points a sender at a local test server and returns the
// captured request form.
func newStubbedSender(t *testing.T, cfg TwilioConfig, status int, body string) (*TwilioWhatsAppSender, *url.Values, *string) {
	t.Helper()
	captured := &url.Values{}
	authHeader := new(string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		*captured = r.PostForm
		*authHeader = r.Header.Get("Authorization")
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/2010-04-01/Accounts/AC123/Messages.json", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	sender := NewTwilioWhatsAppSender(cfg)
	require.NotNil(t, sender)
	sender.baseURL = srv.URL
	return sender, captured, authHeader
}

func TestTwilioConfigEnabled(t *testing.T) {
	assert.True(t, testTwilioConfig().Enabled())

	for _, tc := range []struct {
		name   string
		mutate func(*TwilioConfig)
	}{
		{"no account sid", func(c *TwilioConfig) { c.AccountSID = "" }},
		{"no auth token", func(c *TwilioConfig) { c.AuthToken = "" }},
		{"no sender number", func(c *TwilioConfig) { c.FromNumber = "" }},
		{"blank account sid", func(c *TwilioConfig) { c.AccountSID = "   " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testTwilioConfig()
			tc.mutate(&cfg)
			assert.False(t, cfg.Enabled())
			assert.Nil(t, NewTwilioWhatsAppSender(cfg), "an unconfigured sender must be nil so callers can fall back")
		})
	}
}

func TestTwilioSendOTPPlainBody(t *testing.T) {
	sender, form, auth := newStubbedSender(t, testTwilioConfig(), http.StatusCreated, `{"sid":"SM1"}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com", Phone: "+5491140510100"}, "123456")
	require.NoError(t, err)

	assert.Equal(t, "whatsapp:+14155238886", form.Get("From"))
	assert.Equal(t, "whatsapp:+5491140510100", form.Get("To"))
	assert.Contains(t, form.Get("Body"), "123456")
	// Without a template configured no content fields are sent.
	assert.Empty(t, form.Get("ContentSid"))
	assert.Empty(t, form.Get("ContentVariables"))
	assert.True(t, strings.HasPrefix(*auth, "Basic "), "Twilio uses basic auth")
}

func TestTwilioSendOTPWithTemplate(t *testing.T) {
	cfg := testTwilioConfig()
	cfg.TemplateSID = "HX0000"
	sender, form, _ := newStubbedSender(t, cfg, http.StatusCreated, `{"sid":"SM1"}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+5491140510100"}, "987654")
	require.NoError(t, err)

	assert.Equal(t, "HX0000", form.Get("ContentSid"))
	// The code travels as template variable "1", not as a free-form body.
	var vars map[string]string
	require.NoError(t, json.Unmarshal([]byte(form.Get("ContentVariables")), &vars))
	assert.Equal(t, "987654", vars["1"])
	assert.Empty(t, form.Get("Body"))
}

func TestTwilioSendOTPSurfacesAPIError(t *testing.T) {
	sender, _, _ := newStubbedSender(t, testTwilioConfig(), http.StatusBadRequest,
		`{"code":63016,"message":"Failed to send freeform message","more_info":"https://twilio.com/docs/errors/63016"}`)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Phone: "+5491140510100"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "63016")
	assert.Contains(t, err.Error(), "Failed to send freeform message")
	// The code and credentials must never leak into the error.
	assert.NotContains(t, err.Error(), "123456")
	assert.NotContains(t, err.Error(), "secret-token")
}

func TestTwilioSendOTPRejectsMissingPhone(t *testing.T) {
	sender := NewTwilioWhatsAppSender(testTwilioConfig())
	require.NotNil(t, sender)

	err := sender.SendOTP(context.Background(),
		port.OTPRecipient{Email: "ana@example.com"}, "123456")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no phone number")
}

func TestTwilioChannel(t *testing.T) {
	assert.Equal(t, "whatsapp", NewTwilioWhatsAppSender(testTwilioConfig()).Channel())
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		dialCode string
		want     string
		wantErr  bool
	}{
		{name: "already e164", raw: "+5491140510100", dialCode: "54", want: "+5491140510100"},
		{name: "strips formatting", raw: "+54 (911) 4051-0100", dialCode: "54", want: "+5491140510100"},
		{name: "adds dial code when missing", raw: "1140510100", dialCode: "54", want: "+541140510100"},
		{name: "does not double the dial code", raw: "541140510100", dialCode: "54", want: "+541140510100"},
		{name: "colombian dial code", raw: "3000010100", dialCode: "57", want: "+573000010100"},
		{name: "dial code prefix respected with plus", raw: "+573000010100", dialCode: "57", want: "+573000010100"},
		{name: "empty is rejected", raw: "", dialCode: "54", wantErr: true},
		{name: "blank is rejected", raw: "   ", dialCode: "54", wantErr: true},
		{name: "no digits is rejected", raw: "not-a-phone", dialCode: "54", wantErr: true},
		{name: "too short is rejected", raw: "+12345", dialCode: "54", wantErr: true},
		{name: "too long is rejected", raw: "+1234567890123456", dialCode: "54", wantErr: true},
		{name: "tolerates dial code with plus", raw: "1140510100", dialCode: "+54", want: "+541140510100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePhone(tt.raw, tt.dialCode)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewOTPSenderUsesTwilioWhenItIsTheOnlyGateway(t *testing.T) {
	sender := NewOTPSender(OTPProviders{Twilio: testTwilioConfig()})
	require.NotNil(t, sender)
	assert.Equal(t, "whatsapp", sender.Channel())
}

func TestNewOTPSenderFallsBackToConsole(t *testing.T) {
	// No Twilio credentials and no Firebase credentials file in the environment.

	sender := NewOTPSender(OTPProviders{})
	require.NotNil(t, sender)
	assert.Equal(t, "console", sender.Channel())
}
