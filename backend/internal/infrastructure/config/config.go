package config

import (
	"os"
	"strconv"
)

type Config struct {
	DB       DBConfig
	Server   ServerConfig
	JWT      JWTConfig
	GCS      GCSConfig
	AI       AIConfig
	Twilio   TwilioConfig
	Kapso    KapsoConfig
	SendGrid SendGridConfig
	// OTPProvider selects the delivery channel: "kapso", "twilio" or
	// "sendgrid". Empty uses whichever is configured.
	OTPProvider     string
	Partners        PartnersConfig
	Country         string
	DefaultIVARate  float64
	LatePenaltyRate float64
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

// SendGridConfig configures one-time codes delivered by email. FromEmail must
// be a verified sender identity in SendGrid.
type SendGridConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
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
// Twilio. With no credentials the OTP sender falls back to Firebase email links
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
}

type AIConfig struct {
	APIKey    string
	Model     string
	Effort    string
	MaxTokens int
}

// Enabled reports whether a prompt-driven analyzer can be built.
func (c AIConfig) Enabled() bool {
	return c.APIKey != ""
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
			APIKey:    getEnv("ANTHROPIC_API_KEY", ""),
			Model:     getEnv("RISK_ANALYSIS_MODEL", "claude-opus-5"),
			Effort:    getEnv("RISK_ANALYSIS_EFFORT", "high"),
			MaxTokens: getEnvInt("RISK_ANALYSIS_MAX_TOKENS", 16000),
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
		},
		OTPProvider: getEnv("OTP_PROVIDER", ""),
		Twilio: TwilioConfig{
			AccountSID:  getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:   getEnv("TWILIO_AUTH_TOKEN", ""),
			FromNumber:  getEnv("TWILIO_WHATSAPP_FROM", ""),
			TemplateSID: getEnv("TWILIO_WHATSAPP_TEMPLATE_SID", ""),
		},
		Partners: PartnersConfig{
			CuppoganAPIKey: getEnv("CUPPOGAN_API_KEY", ""),
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
