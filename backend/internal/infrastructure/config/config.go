package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

type Config struct {
	DB       DBConfig
	Server   ServerConfig
	JWT      JWTConfig
	GCS      GCSConfig
	AI       AIConfig
	Twilio   TwilioConfig
	Kapso    KapsoConfig
	RUV      RUVConfig
	Google   GoogleOAuthConfig
	SendGrid SendGridConfig
	// OTPProvider selects the delivery channel: "kapso", "twilio" or
	// "sendgrid". Empty uses whichever is configured.
	OTPProvider string
	Partners    PartnersConfig
	// Odoo is TVGAN's Odoo, the ledger of the livestock credit portfolio.
	Odoo            OdooConfig
	Country         string
	DefaultIVARate  float64
	LatePenaltyRate float64
	// LivestockQuota selects how livestock quotas are decided.
	LivestockQuota LivestockQuotaConfig
}

// CountryInfo holds the deployment-country localization metadata that the
// frontend consumes via the public /config endpoint.
type CountryInfo struct {
	Country         string  `json:"country"`
	Currency        string  `json:"currency"`
	Locale          string  `json:"locale"`
	NationalIDLabel string  `json:"nationalIdLabel"`
	TaxIDLabel      string  `json:"taxIdLabel"`
	IVARate         float64 `json:"ivaRate"`
}

// countryDefaults maps a country code to its localization defaults.
var countryDefaults = map[string]struct {
	Currency        string
	Locale          string
	TimeZone        string
	NationalIDLabel string
	TaxIDLabel      string
	IVARate         float64
	DialCode        string
}{
	"AR": {Currency: "ARS", Locale: "es-AR", TimeZone: "America/Argentina/Buenos_Aires", NationalIDLabel: "DNI", TaxIDLabel: "CUIT", IVARate: 21, DialCode: "54"},
	"CO": {Currency: "COP", Locale: "es-CO", TimeZone: "America/Bogota", NationalIDLabel: "Cédula", TaxIDLabel: "NIT", IVARate: 19, DialCode: "57"},
}

// DialCode is the deployment country's international dialing code, used to put
// stored phone numbers into E.164 before handing them to Twilio.
func (c *Config) DialCode() string { return countryDefaults[c.Country].DialCode }

// CountryInfo returns the localization metadata for the configured country.
func (c *Config) CountryInfo() CountryInfo {
	d := countryDefaults[c.Country]
	return CountryInfo{
		Country:         c.Country,
		Currency:        d.Currency,
		Locale:          d.Locale,
		NationalIDLabel: d.NationalIDLabel,
		TaxIDLabel:      d.TaxIDLabel,
		IVARate:         c.DefaultIVARate,
	}
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	Instance string
	TimeZone string
}

func (c DBConfig) DSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	tz := c.TimeZone
	if tz == "" {
		tz = "America/Argentina/Buenos_Aires"
	}
	dsn := "host=" + c.Host +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Name +
		" port=" + c.Port +
		" sslmode=" + sslMode +
		" TimeZone=" + tz
	return dsn
}

type ServerConfig struct {
	Port string
	Mode string
}

type JWTConfig struct {
	Secret          string
	ExpirationHours int
}

// GoogleOAuthConfig enables administrator sign-in with Google. The client id is
// public: it ships in the browser bundle by design. HostedDomain, when set,
// restricts sign-in to one Workspace domain.
type GoogleOAuthConfig struct {
	ClientID     string
	HostedDomain string
}

// RUVConfig points at Fedegán's vaccination registry. With no credentials the
// quota engine still scores herds a caller supplies; it just cannot read them.
type RUVConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
}

// LivestockQuotaConfig selects the livestock quota strategy. The values are
// kept raw here and parsed where the service is built, which refuses to start
// on an invalid one rather than grant quotas under the wrong rule.
type LivestockQuotaConfig struct {
	// Strategy is "tiers" (the pilot's fixed amounts, the default when empty)
	// or "engine" (the individual engine).
	Strategy string
	// CurrentCycle is the RUV cycle the tiers measure, as "YYYY-C"; empty
	// means the built-in default.
	CurrentCycle string
}

// SendGridConfig configures one-time codes delivered by email. FromEmail must
// be a verified sender identity in SendGrid.
type SendGridConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
	LogoURL   string
}

// KapsoConfig configures WhatsApp delivery through Kapso, which fronts the
// WhatsApp Cloud API with Meta's own request shape. PhoneNumberID is the
// connected number's id, not the number itself.
type KapsoConfig struct {
	APIKey           string
	PhoneNumberID    string
	TemplateName     string
	TemplateLanguage string
}

// TwilioConfig configures WhatsApp delivery of one-time login codes through
// Twilio. With no credentials the OTP sender falls back to another configured
// and then to the console, so local development needs no Twilio account.
type TwilioConfig struct {
	AccountSID  string
	AuthToken   string
	FromNumber  string
	TemplateSID string
}

// AIConfig configures the language model that runs the credit-risk analysis
// prompt. With no API key the risk module falls back to its deterministic
// scorecard, so the feature works without any AI credentials.
// PartnersConfig holds the keys partner services authenticate with. An empty
// key disables that partner's routes.
type PartnersConfig struct {
	// CuppoganAPIKey authenticates CuppoGan on POST /livestock-quotas.
	CuppoganAPIKey string
	// LivestockCreditRateTEA (tasa efectiva anual) and
	// LivestockCreditMaxInstallments are the terms of the credit line each
	// CuppoGan activation opens in Prestia. Each quota invoice in Odoo becomes
	// a loan on it at that rate (installments: Odoo's "Cuppogan" term, 3).
	LivestockCreditRateTEA         decimal.Decimal
	LivestockCreditMaxInstallments int
	// SchedulerAPIKey authenticates Cloud Scheduler on the unattended Odoo
	// sync (POST /livestock-credit-portfolio/odoo/sync).
	SchedulerAPIKey string
}

// OdooConfig points at TVGAN's Odoo API for CuppoGan (/api/v1/cuppogan),
// where the purchases on the livestock quota are invoiced and collected. With
// no token the portfolio features are off and the stored data stays readable.
type OdooConfig struct {
	BaseURL string
	Token   string
	// TimeoutSeconds bounds each request.
	TimeoutSeconds int
	// RequestsPerMinute and RequestsPerHour throttle Prestia client-side:
	// Odoo allows 60/min and 600/h per IP, shared with CuppoGan's service.
	RequestsPerMinute int
	RequestsPerHour   int
	// SyncIntervalMinutes is how often every line is refreshed from Odoo;
	// 0 turns the periodic sync off (the on-demand button still works).
	SyncIntervalMinutes int
}

// AIConfig selects and configures the provider behind the risk analysis.
type AIConfig struct {
	// Provider is "gemini" (Vertex AI, the default) or "claude" (the
	// Anthropic API).
	Provider string
	// APIKey is the Anthropic API key, used only by the claude provider.
	APIKey string
	// VertexProject and VertexLocation are where the gemini provider calls
	// Vertex AI. It authenticates as the runtime service account, so it needs
	// no key.
	VertexProject  string
	VertexLocation string
	// Model empty means the provider's default model.
	Model     string
	Effort    string
	MaxTokens int
}

// Enabled reports whether a prompt-driven analyzer can be built.
func (c AIConfig) Enabled() bool {
	if c.Provider == "claude" {
		return c.APIKey != ""
	}
	return c.VertexProject != ""
}

type GCSConfig struct {
	Bucket          string
	CredentialsFile string
	LocalPath       string
}

func Load() *Config {
	country := getEnv("COUNTRY", "AR")
	defaults, ok := countryDefaults[country]
	if !ok {
		// Unknown country falls back to Argentina defaults.
		country = "AR"
		defaults = countryDefaults["AR"]
	}

	return &Config{
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "creditos"),
			Password: getEnv("DB_PASSWORD", "creditos_secret"),
			Name:     getEnv("DB_NAME", "creditos"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			Instance: getEnv("DB_INSTANCE", ""),
			TimeZone: defaults.TimeZone,
		},
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Mode: getEnv("GIN_MODE", "debug"),
		},
		JWT: JWTConfig{
			Secret:          getEnv("JWT_SECRET", "change-me-in-production"),
			ExpirationHours: getEnvInt("JWT_EXPIRATION_HOURS", 24),
		},
		GCS: GCSConfig{
			Bucket:          getEnv("GCS_BUCKET", ""),
			CredentialsFile: getEnv("GCS_CREDENTIALS_FILE", ""),
			LocalPath:       getEnv("LOCAL_STORAGE_PATH", "./storage"),
		},
		AI: AIConfig{
			Provider:       strings.ToLower(getEnv("RISK_ANALYSIS_PROVIDER", "gemini")),
			APIKey:         getEnv("ANTHROPIC_API_KEY", ""),
			VertexProject:  getEnv("VERTEX_PROJECT", getEnv("GOOGLE_CLOUD_PROJECT", "")),
			VertexLocation: getEnv("VERTEX_LOCATION", ""),
			Model:          getEnv("RISK_ANALYSIS_MODEL", ""),
			Effort:         getEnv("RISK_ANALYSIS_EFFORT", "high"),
			MaxTokens:      getEnvInt("RISK_ANALYSIS_MAX_TOKENS", 16000),
		},
		Kapso: KapsoConfig{
			APIKey:           getEnv("KAPSO_API_KEY", ""),
			PhoneNumberID:    getEnv("KAPSO_PHONE_NUMBER_ID", ""),
			TemplateName:     getEnv("KAPSO_TEMPLATE_NAME", ""),
			TemplateLanguage: getEnv("KAPSO_TEMPLATE_LANGUAGE", ""),
		},
		SendGrid: SendGridConfig{
			APIKey:    getEnv("SENDGRID_API_KEY", ""),
			FromEmail: getEnv("SENDGRID_FROM_EMAIL", ""),
			FromName:  getEnv("SENDGRID_FROM_NAME", "CuppoGan"),
			LogoURL:   getEnv("SENDGRID_LOGO_URL", ""),
		},
		RUV: RUVConfig{
			BaseURL:      getEnv("RUV_BASE_URL", ""),
			ClientID:     getEnv("RUV_CLIENT_ID", ""),
			ClientSecret: getEnv("RUV_CLIENT_SECRET", ""),
			Username:     getEnv("RUV_USERNAME", ""),
			Password:     getEnv("RUV_PASSWORD", ""),
		},
		LivestockQuota: LivestockQuotaConfig{
			Strategy:     getEnv("LIVESTOCK_QUOTA_STRATEGY", ""),
			CurrentCycle: getEnv("LIVESTOCK_QUOTA_CURRENT_CYCLE", ""),
		},
		Google: GoogleOAuthConfig{
			ClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
			HostedDomain: getEnv("GOOGLE_HOSTED_DOMAIN", ""),
		},
		OTPProvider: getEnv("OTP_PROVIDER", ""),
		Twilio: TwilioConfig{
			AccountSID:  getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:   getEnv("TWILIO_AUTH_TOKEN", ""),
			FromNumber:  getEnv("TWILIO_WHATSAPP_FROM", ""),
			TemplateSID: getEnv("TWILIO_WHATSAPP_TEMPLATE_SID", ""),
		},
		Partners: PartnersConfig{
			CuppoganAPIKey:  getEnv("CUPPOGAN_API_KEY", ""),
			SchedulerAPIKey: getEnv("SCHEDULER_API_KEY", ""),
			// CuppoGan's rate: 26.01 % TEA.
			LivestockCreditRateTEA:         decimal.NewFromFloat(getEnvFloat("LIVESTOCK_CREDIT_RATE_TEA", 26.01)),
			LivestockCreditMaxInstallments: getEnvInt("LIVESTOCK_CREDIT_MAX_INSTALLMENTS", 3),
		},
		Odoo: OdooConfig{
			BaseURL:             getEnv("ODOO_API_BASE_URL", ""),
			Token:               getEnv("ODOO_API_TOKEN", ""),
			TimeoutSeconds:      getEnvInt("ODOO_API_TIMEOUT_SECONDS", 15),
			RequestsPerMinute:   getEnvInt("ODOO_API_REQUESTS_PER_MINUTE", 15),
			RequestsPerHour:     getEnvInt("ODOO_API_REQUESTS_PER_HOUR", 150),
			SyncIntervalMinutes: getEnvInt("ODOO_SYNC_INTERVAL_MINUTES", 5),
		},
		Country:         country,
		DefaultIVARate:  getEnvFloat("DEFAULT_IVA_RATE", defaults.IVARate),
		LatePenaltyRate: getEnvFloat("LATE_PENALTY_RATE", 10),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
