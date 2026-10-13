package handler

import (
	"net/http"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/auth"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	clientService  *service.ClientService
	userRepo       port.UserRepository
	authService    *auth.LocalAuthService
	otpService     *service.OTPService
	googleVerifier *auth.GoogleVerifier
}

func NewAuthHandler(
	clientService *service.ClientService,
	userRepo port.UserRepository,
	authService *auth.LocalAuthService,
	otpService *service.OTPService,
	googleVerifier *auth.GoogleVerifier,
) *AuthHandler {
	return &AuthHandler{
		clientService:  clientService,
		userRepo:       userRepo,
		authService:    authService,
		otpService:     otpService,
		googleVerifier: googleVerifier,
	}
}

// Register godoc
// @Summary Register a new client
// @Description Creates a new user account and client profile with the provided personal information
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.RegisterRequest true "Registration data"
// @Success 201 {object} dto.AuthResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	client, user, err := h.clientService.Register(
		c.Request.Context(), req.Email,
		req.FirstName, req.LastName, req.DNI, req.CUIT, req.DateOfBirth,
		req.Phone, req.Address, req.City, req.Province, req.Country, req.IsPEP,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	token, err := h.authService.GenerateToken(user.FirebaseUID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to generate token"})
		return
	}

	_ = client
	c.JSON(http.StatusCreated, dto.AuthResponse{
		Token: token,
		User:  dto.ToUserResponse(user),
	})
}

// RequestOTP godoc
// @Summary Request an OTP code for client/vendor login
// @Description Sends a 6-digit OTP code via email or signals phone OTP (sent client-side by Firebase)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.RequestOTPRequest true "Email/phone and channel"
// @Success 200 {object} map[string]string
// @Failure 400 {object} dto.ErrorResponse
// @Router /auth/request-otp [post]
func (h *AuthHandler) RequestOTP(c *gin.Context) {
	var req dto.RequestOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	channel := req.Channel
	if channel == "" {
		channel = "email"
	}

	if channel == "phone" {
		// For phone channel, validate the user exists but SMS is sent client-side by Firebase
		if req.Phone == "" {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "phone is required for phone channel"})
			return
		}
		_, err := h.userRepo.FindByPhone(c.Request.Context(), req.Phone)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "user not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Send SMS via Firebase client SDK", "channel": "phone"})
		return
	}

	// Email channel: existing flow
	if req.Email == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "email is required for email channel"})
		return
	}

	if err := h.otpService.RequestOTP(c.Request.Context(), req.Email); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	ch := h.otpService.Channel()
	c.JSON(http.StatusOK, gin.H{"message": "OTP sent via " + ch, "channel": ch})
}

// VerifyOTP godoc
// @Summary Verify an OTP code and login
// @Description Validates the OTP code and returns a JWT token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.VerifyOTPRequest true "Email and OTP code"
// @Success 200 {object} dto.AuthResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/verify-otp [post]
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req dto.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.otpService.VerifyOTP(c.Request.Context(), req.Email, req.Code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: err.Error()})
		return
	}

	token, err := h.authService.GenerateToken(user.FirebaseUID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, dto.AuthResponse{
		Token: token,
		User:  dto.ToUserResponse(user),
	})
}

// GoogleLogin godoc
// @Summary Sign in an administrator with Google
// @Description Verifies a Google ID token and issues a session for an existing administrator. Accounts are never created here.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.GoogleLoginRequest true "Google ID token"
// @Success 200 {object} dto.AuthResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 503 {object} dto.ErrorResponse "Google sign-in not configured"
// @Router /auth/google [post]
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	if h.googleVerifier == nil {
		c.JSON(http.StatusServiceUnavailable, dto.ErrorResponse{Error: "Google sign-in is not configured"})
		return
	}

	var req dto.GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	identity, err := h.googleVerifier.Verify(c.Request.Context(), req.IDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// A valid Google token proves who the person is, not that they may enter.
	// The account must already exist here: nothing is created from a sign-in,
	// or anyone with a Google account could grant themselves access.
	user, err := h.userRepo.FindByEmail(c.Request.Context(), identity.Email)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "this account has no access"})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "account is deactivated"})
		return
	}
	if !user.IsAdmin() {
		// Google sign-in is the administration door. Clients and vendors use
		// the one-time code, which reaches them where they actually are.
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Google sign-in is for administrators"})
		return
	}

	token, err := h.authService.GenerateToken(user.FirebaseUID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to generate token"})
		return
	}

	user.RecordLogin()
	_ = h.userRepo.Update(c.Request.Context(), user)

	c.JSON(http.StatusOK, dto.AuthResponse{Token: token, User: dto.ToUserResponse(user)})
}
