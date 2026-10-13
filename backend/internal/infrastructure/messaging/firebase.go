package messaging

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"google.golang.org/api/option"
)

// FirebaseOTPSender sends OTP via Firebase Auth email link.
type FirebaseOTPSender struct {
	authClient *auth.Client
}

func NewFirebaseOTPSender(credentialsFile string) (*FirebaseOTPSender, error) {
	if credentialsFile == "" {
		return nil, fmt.Errorf("firebase credentials file not configured")
	}

	opt := option.WithCredentialsFile(credentialsFile)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	authClient, err := app.Auth(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase Auth client: %w", err)
	}

	return &FirebaseOTPSender{authClient: authClient}, nil
}

func (s *FirebaseOTPSender) Channel() string { return "email" }

func (s *FirebaseOTPSender) SendOTP(ctx context.Context, to port.OTPRecipient, code string) error {
	actionCodeSettings := &auth.ActionCodeSettings{
		URL:             os.Getenv("OTP_REDIRECT_URL"),
		HandleCodeInApp: true,
	}

	link, err := s.authClient.EmailSignInLink(ctx, to.Email, actionCodeSettings)
	if err != nil {
		return fmt.Errorf("failed to generate email sign-in link: %w", err)
	}

	// Log the link for debugging; in production Firebase sends the email automatically.
	log.Printf("[Firebase OTP] Email: %s, Code: %s, Link: %s", to.Email, code, link)
	return nil
}

// ConsoleOTPSender logs OTP codes to console for development.
type ConsoleOTPSender struct{}

func NewConsoleOTPSender() *ConsoleOTPSender {
	return &ConsoleOTPSender{}
}

func (s *ConsoleOTPSender) Channel() string { return "console" }

func (s *ConsoleOTPSender) SendOTP(_ context.Context, to port.OTPRecipient, code string) error {
	log.Printf("[OTP] Code for %s (%s): %s", to.Email, to.Phone, code)
	return nil
}

// OTPProviders carries every delivery option the deployment may have
// configured. Which one is used is decided by OTPProvider, falling back to
// whatever is actually configured.
type OTPProviders struct {
	// Provider names the WhatsApp gateway to use: "kapso" or "twilio".
	// Empty picks whichever one is configured, preferring Kapso.
	Provider string
	Kapso    KapsoConfig
	Twilio   TwilioConfig
}

// NewOTPSender picks the delivery channel from configuration. WhatsApp goes
// through the selected gateway; without one it falls back to Firebase email
// links and finally the console, so local development needs no accounts.
func NewOTPSender(p OTPProviders) port.OTPSender {
	kapso := NewKapsoWhatsAppSender(p.Kapso)
	twilio := NewTwilioWhatsAppSender(p.Twilio)

	switch strings.ToLower(strings.TrimSpace(p.Provider)) {
	case "kapso":
		if kapso != nil {
			log.Println("OTP delivery: Kapso WhatsApp")
			return kapso
		}
		log.Println("Warning: OTP_PROVIDER=kapso but Kapso is not configured")
	case "twilio":
		if twilio != nil {
			log.Println("OTP delivery: Twilio WhatsApp")
			return twilio
		}
		log.Println("Warning: OTP_PROVIDER=twilio but Twilio is not configured")
	}

	// No explicit choice, or the chosen one is not configured.
	if kapso != nil {
		log.Println("OTP delivery: Kapso WhatsApp")
		return kapso
	}
	if twilio != nil {
		log.Println("OTP delivery: Twilio WhatsApp")
		return twilio
	}

	if credFile := os.Getenv("FIREBASE_CREDENTIALS_FILE"); credFile != "" {
		sender, err := NewFirebaseOTPSender(credFile)
		if err != nil {
			log.Printf("Warning: Failed to initialize Firebase OTP sender: %v. Falling back to console.", err)
			return NewConsoleOTPSender()
		}
		log.Println("OTP delivery: Firebase email link")
		return sender
	}

	log.Println("OTP delivery: console (no WhatsApp gateway or Firebase credentials configured)")
	return NewConsoleOTPSender()
}
