package messaging

import (
	"context"
	"log"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// ConsoleOTPSender logs codes instead of sending them, so local development
// needs no provider account.
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
// configured. Which one is used is decided by Provider, falling back to
// whatever is actually configured.
type OTPProviders struct {
	// Provider names the channel to use: "kapso", "twilio" or "sendgrid".
	// Empty picks whichever is configured, preferring WhatsApp over email.
	Provider string
	Kapso    KapsoConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
}

// NewOTPSender picks the delivery channel from configuration: the named
// gateway when it is configured, otherwise whichever one is, preferring
// WhatsApp over email, and finally the console.
//
// Naming a provider that is not configured falls through to one that works
// rather than silently sending nothing, which is the worst possible outcome
// for a login code.
func NewOTPSender(p OTPProviders) port.OTPSender {
	kapso := NewKapsoWhatsAppSender(p.Kapso)
	twilio := NewTwilioWhatsAppSender(p.Twilio)
	sendgrid := NewSendGridOTPSender(p.SendGrid)

	switch strings.ToLower(strings.TrimSpace(p.Provider)) {
	case "sendgrid":
		if sendgrid != nil {
			log.Println("OTP delivery: SendGrid email")
			return sendgrid
		}
		log.Println("Warning: OTP_PROVIDER=sendgrid but SendGrid is not configured")
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
	if sendgrid != nil {
		log.Println("OTP delivery: SendGrid email")
		return sendgrid
	}

	log.Println("OTP delivery: console (no gateway configured)")
	return NewConsoleOTPSender()
}
