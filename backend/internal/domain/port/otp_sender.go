package port

import "context"

// OTPRecipient is where a one-time code can be delivered. The application
// resolves both addresses from the user record and hands them to the adapter,
// so each channel picks the one it needs: WhatsApp uses Phone, email senders
// use Email.
type OTPRecipient struct {
	Email string
	Phone string
}

// OTPSender delivers a one-time login code. Implementations are infrastructure
// adapters (Twilio WhatsApp in production, console in development).
type OTPSender interface {
	// Channel names the delivery channel for audit and log messages.
	Channel() string
	SendOTP(ctx context.Context, to OTPRecipient, code string) error
}
