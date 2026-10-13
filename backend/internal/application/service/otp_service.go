package service

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

type OTPService struct {
	otpRepo  port.OTPRepository
	userRepo port.UserRepository
	sender   port.OTPSender
	audit    *AuditService
}

func NewOTPService(
	otpRepo port.OTPRepository,
	userRepo port.UserRepository,
	sender port.OTPSender,
	audit *AuditService,
) *OTPService {
	return &OTPService{
		otpRepo:  otpRepo,
		userRepo: userRepo,
		sender:   sender,
		audit:    audit,
	}
}

func (s *OTPService) RequestOTP(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Every role goes through the one-time code, administrators included.
	// Admins used to be excluded because they signed in through the
	// credential-less /auth/login shortcut, which has been removed: a code
	// delivered out of band is now the only non-Firebase way in.

	if !user.IsActive {
		return fmt.Errorf("account is deactivated")
	}

	// Invalidate previous unused OTPs
	_ = s.otpRepo.DeleteExpiredByEmail(ctx, email)

	// Generate new OTP
	otp, err := model.NewOTPCode(email)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	if err := s.otpRepo.Create(ctx, otp); err != nil {
		return fmt.Errorf("failed to store OTP: %w", err)
	}

	// Send the code over the configured channel. WhatsApp needs a phone on
	// file, so fail loudly rather than silently storing a code nobody receives.
	to := port.OTPRecipient{Email: user.Email, Phone: user.Phone}
	if err := s.sender.SendOTP(ctx, to, otp.Code); err != nil {
		return fmt.Errorf("failed to send OTP: %w", err)
	}

	s.audit.Record(ctx, &user.ID, "request_otp", "user", user.ID.String(),
		fmt.Sprintf("OTP requested for %s via %s", email, s.sender.Channel()))

	return nil
}

func (s *OTPService) VerifyOTP(ctx context.Context, email, code string) (*model.User, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("account is deactivated")
	}

	otp, err := s.otpRepo.FindLatestByEmail(ctx, email)
	if err != nil {
		s.audit.Record(ctx, &user.ID, "otp_verify_failed", "user", user.ID.String(), "No OTP found")
		return nil, fmt.Errorf("invalid or expired code")
	}

	if !otp.IsValid(code) {
		s.audit.Record(ctx, &user.ID, "otp_verify_failed", "user", user.ID.String(), "Invalid OTP code")
		return nil, fmt.Errorf("invalid or expired code")
	}

	// Mark OTP as used
	otp.MarkUsed()
	if err := s.otpRepo.MarkUsed(ctx, otp.ID); err != nil {
		return nil, fmt.Errorf("failed to mark OTP as used: %w", err)
	}

	// Record login
	user.RecordLogin()
	_ = s.userRepo.Update(ctx, user)

	s.audit.Record(ctx, &user.ID, "otp_login", "user", user.ID.String(), fmt.Sprintf("Client %s logged in via OTP", email))

	return user, nil
}
