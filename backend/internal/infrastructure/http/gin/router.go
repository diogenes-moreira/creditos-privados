package gin

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/ai"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/auth"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/config"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/messaging"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/pdf"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/persistence/postgres"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/ruv"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/storage"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

type Router struct {
	engine          *gin.Engine
	db              *gorm.DB
	defaultIVARate  float64
	latePenaltyRate decimal.Decimal
	country         string
	countryInfo     config.CountryInfo
	aiConfig        config.AIConfig
	ruvConfig       config.RUVConfig
	googleConfig    config.GoogleOAuthConfig
	otpProviders    messaging.OTPProviders
	partners        config.PartnersConfig
}

func NewRouter(db *gorm.DB, cfg *config.Config) *Router {
	r := &Router{
		engine:          gin.Default(),
		db:              db,
		defaultIVARate:  cfg.DefaultIVARate,
		latePenaltyRate: decimal.NewFromFloat(cfg.LatePenaltyRate),
		country:         cfg.Country,
		countryInfo:     cfg.CountryInfo(),
		aiConfig:        cfg.AI,
		ruvConfig:       cfg.RUV,
		googleConfig:    cfg.Google,
		partners:        cfg.Partners,
		otpProviders: messaging.OTPProviders{
			Provider: cfg.OTPProvider,
			Kapso: messaging.KapsoConfig{
				APIKey:           cfg.Kapso.APIKey,
				PhoneNumberID:    cfg.Kapso.PhoneNumberID,
				TemplateName:     cfg.Kapso.TemplateName,
				TemplateLanguage: cfg.Kapso.TemplateLanguage,
				DefaultDialCode:  cfg.DialCode(),
			},
			SendGrid: messaging.SendGridConfig{
				APIKey:    cfg.SendGrid.APIKey,
				FromEmail: cfg.SendGrid.FromEmail,
				FromName:  cfg.SendGrid.FromName,
			},
			Twilio: messaging.TwilioConfig{
				AccountSID:      cfg.Twilio.AccountSID,
				AuthToken:       cfg.Twilio.AuthToken,
				FromNumber:      cfg.Twilio.FromNumber,
				TemplateSID:     cfg.Twilio.TemplateSID,
				DefaultDialCode: cfg.DialCode(),
			},
		},
	}
	r.engine.Use(middleware.CORS())
	r.engine.Use(middleware.AuditContext())
	r.setupRoutes(cfg.JWT.Secret)
	return r
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}

func (r *Router) setupRoutes(jwtSecret string) {
	// Repositories
	userRepo := postgres.NewUserRepository(r.db)
	clientRepo := postgres.NewClientRepository(r.db)
	accountRepo := postgres.NewAccountRepository(r.db)
	movementRepo := postgres.NewMovementRepository(r.db)
	creditLineRepo := postgres.NewCreditLineRepository(r.db)
	loanRepo := postgres.NewLoanRepository(r.db)
	installmentRepo := postgres.NewInstallmentRepository(r.db)
	paymentRepo := postgres.NewPaymentRepository(r.db)
	auditLogRepo := postgres.NewAuditLogRepository(r.db)
	dashboardRepo := postgres.NewDashboardRepository(r.db)
	vendorRepo := postgres.NewVendorRepository(r.db)
	vendorAccountRepo := postgres.NewVendorAccountRepository(r.db)
	vendorMovementRepo := postgres.NewVendorMovementRepository(r.db)
	purchaseRepo := postgres.NewPurchaseRepository(r.db)
	vendorPaymentRepo := postgres.NewVendorPaymentRepository(r.db)
	otpRepo := postgres.NewOTPRepository(r.db)
	riskProfileRepo := postgres.NewRiskProfileRepository(r.db)
	riskModelRepo := postgres.NewRiskModelRepository(r.db)
	riskAssessmentRepo := postgres.NewRiskAssessmentRepository(r.db)
	creditHistoryRepo := postgres.NewCreditHistoryRepository(r.db)
	livestockQuotaRepo := postgres.NewLivestockQuotaRepository(r.db)
	livestockPolicyRepo := postgres.NewLivestockQuotaPolicyRepository(r.db)

	// Adapters
	authService := auth.NewLocalAuthService(jwtSecret, 24)
	otpSender := messaging.NewOTPSender(r.otpProviders)
	localStorage := storage.NewLocalStorage("./storage")
	pdfGenerator := pdf.NewGenerator()

	// Application services
	auditService := service.NewAuditService(auditLogRepo)
	clientService := service.NewClientService(userRepo, clientRepo, accountRepo, authService, auditService, r.country)
	creditService := service.NewCreditService(creditLineRepo, loanRepo, installmentRepo, accountRepo, movementRepo, auditService, clientRepo, paymentRepo)
	paymentService := service.NewPaymentService(paymentRepo, loanRepo, installmentRepo, accountRepo, movementRepo, auditService, r.latePenaltyRate)
	accountService := service.NewAccountService(accountRepo, movementRepo)
	dashboardService := service.NewDashboardService(dashboardRepo)
	_ = service.NewPDFAppService(pdfGenerator, localStorage, loanRepo, clientRepo, paymentRepo)
	vendorService := service.NewVendorService(userRepo, vendorRepo, vendorAccountRepo, authService, auditService, r.country)
	purchaseService := service.NewPurchaseService(purchaseRepo, vendorRepo, vendorAccountRepo, vendorMovementRepo, clientRepo, creditService, auditService)
	vendorPaymentService := service.NewVendorPaymentService(vendorPaymentRepo, vendorAccountRepo, vendorMovementRepo, vendorRepo, auditService)
	otpService := service.NewOTPService(otpRepo, userRepo, otpSender, auditService)
	withdrawalRepo := postgres.NewWithdrawalRequestRepository(r.db)
	withdrawalService := service.NewWithdrawalService(withdrawalRepo, vendorRepo, vendorAccountRepo, vendorPaymentRepo, vendorMovementRepo, auditService)
	reportRepo := postgres.NewReportRepository(r.db)
	reportService := service.NewReportService(reportRepo)

	// Credit risk. The analyzer is nil when no AI credentials are configured,
	// in which case the risk service scores with its deterministic scorecard.
	var riskAnalyzer port.RiskAnalyzer
	if analyzer := ai.NewClaudeAnalyzer(ai.Config{
		APIKey:    r.aiConfig.APIKey,
		Model:     r.aiConfig.Model,
		Effort:    r.aiConfig.Effort,
		MaxTokens: r.aiConfig.MaxTokens,
	}); analyzer != nil {
		riskAnalyzer = analyzer
	}
	riskService := service.NewRiskService(
		riskProfileRepo, riskModelRepo, riskAssessmentRepo, creditHistoryRepo,
		clientRepo, creditLineRepo, creditService, auditService, riskAnalyzer,
		r.country, r.countryInfo.Currency, decimal.NewFromFloat(r.defaultIVARate),
	)

	// Handlers
	healthHandler := handler.NewHealthHandler(r.db)
	configHandler := handler.NewConfigHandler(r.countryInfo)
	authHandler := handler.NewAuthHandler(clientService, userRepo, authService, otpService,
		auth.NewGoogleVerifierOrNil(r.googleConfig.ClientID, r.googleConfig.HostedDomain),
		auditService)
	clientHandler := handler.NewClientHandler(clientService, creditService, paymentService, purchaseRepo, accountRepo, movementRepo)
	accountHandler := handler.NewAccountHandler(accountService, clientRepo)
	creditHandler := handler.NewCreditHandler(creditService)
	loanHandler := handler.NewLoanHandler(creditService, paymentService, clientRepo, r.defaultIVARate, pdfGenerator)
	paymentHandler := handler.NewPaymentHandler(paymentService, clientRepo)
	dashboardHandler := handler.NewDashboardHandler(dashboardService)
	auditHandler := handler.NewAuditHandler(auditService)
	userHandler := handler.NewUserHandler(service.NewUserService(userRepo, clientRepo, vendorRepo, auditService))
	vendorHandler := handler.NewVendorHandler(vendorService, purchaseService, vendorPaymentService, withdrawalService, pdfGenerator, vendorRepo, vendorAccountRepo, vendorMovementRepo, clientService, creditService)
	reportHandler := handler.NewReportHandler(reportService)
	riskHandler := handler.NewRiskHandler(riskService, clientRepo)
	livestockPolicyService := service.NewLivestockQuotaPolicyService(livestockPolicyRepo, auditService)
	livestockQuotaService := service.NewLivestockQuotaService(livestockQuotaRepo, r.countryInfo.Currency).
		WithPolicies(livestockPolicyService)
	// The registry is nil when no credentials are configured, in which case the
	// engine still scores herds a caller supplies; it just cannot read them.
	if registry := ruv.New(ruv.Config{
		BaseURL:      r.ruvConfig.BaseURL,
		ClientID:     r.ruvConfig.ClientID,
		ClientSecret: r.ruvConfig.ClientSecret,
		Username:     r.ruvConfig.Username,
		Password:     r.ruvConfig.Password,
	}); registry != nil {
		livestockQuotaService = livestockQuotaService.WithRegistry(registry)
	}
	livestockQuotaHandler := handler.NewLivestockQuotaHandler(livestockQuotaService)
	livestockPolicyHandler := handler.NewLivestockQuotaPolicyHandler(livestockPolicyService)

	// Optional path prefix for when the service is mounted under a sub-path.
	// Empty by default: the app is served from the root.
	basePath := os.Getenv("BASE_PATH")

	// Health (always at root for probes)
	r.engine.GET("/health", healthHandler.Health)
	r.engine.GET("/health/ready", healthHandler.Ready)

	base := r.engine.Group(basePath)

	// Swagger
	base.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := base.Group("/api/v1")

	// Public deployment config (country, currency, locale, identity labels)
	api.GET("/config", configHandler.GetConfig)

	// Auth (public)
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/request-otp", authHandler.RequestOTP)
	api.POST("/auth/verify-otp", authHandler.VerifyOTP)
	api.POST("/auth/google", authHandler.GoogleLogin)

	// Partner services (service key, not a user session). Not registered when
	// the partner's key is not configured.
	if r.partners.CuppoganAPIKey != "" {
		api.POST("/livestock-quotas", middleware.RequireServiceKey("cuppogan", r.partners.CuppoganAPIKey),
			livestockQuotaHandler.Assess)
	} else {
		log.Println("CUPPOGAN_API_KEY not set: POST /livestock-quotas disabled")
	}

	// Authenticated routes
	authenticated := api.Group("")
	authenticated.Use(middleware.AuthMiddleware(authService))

	// Client routes
	clientRoutes := authenticated.Group("")
	clientRoutes.Use(middleware.RequireRole(userRepo, model.RoleClient, model.RoleAdmin))
	{
		clientRoutes.GET("/me/profile", clientHandler.GetProfile)
		clientRoutes.PUT("/me/profile", clientHandler.UpdateProfile)
		clientRoutes.PUT("/me/mercadopago", clientHandler.SetMercadoPagoLink)
		clientRoutes.GET("/me/account", accountHandler.GetAccount)
		clientRoutes.GET("/me/account/movements", accountHandler.GetMovements)
		clientRoutes.GET("/me/loans", loanHandler.GetMyLoans)
		clientRoutes.GET("/me/loans/:id", loanHandler.GetLoanDetail)
		clientRoutes.POST("/me/loans", loanHandler.RequestLoan)
		clientRoutes.POST("/me/loans/:loanId/payments", paymentHandler.RecordPayment)
		clientRoutes.GET("/me/payments", paymentHandler.GetMyPayments)
		clientRoutes.GET("/me/risk-profile", riskHandler.GetMyRiskProfile)
		clientRoutes.PUT("/me/risk-profile", riskHandler.SubmitMyRiskProfile)
		clientRoutes.GET("/me/credit-availability", riskHandler.GetMyCreditAvailability)
	}

	// Loan simulation (any authenticated user)
	authenticated.POST("/loans/simulate", loanHandler.Simulate)

	// Credit-risk pre-qualification (any authenticated user, nothing stored)
	authenticated.POST("/risk/simulate", riskHandler.SimulateRisk)

	// Vendor routes
	vendorRoutes := authenticated.Group("/me/vendor")
	vendorRoutes.Use(middleware.RequireRole(userRepo, model.RoleVendor))
	{
		vendorRoutes.GET("/profile", vendorHandler.GetProfile)
		vendorRoutes.PUT("/profile", vendorHandler.UpdateProfile)
		vendorRoutes.GET("/account", vendorHandler.GetAccount)
		vendorRoutes.GET("/account/movements", vendorHandler.GetMovements)
		vendorRoutes.GET("/purchases", vendorHandler.GetPurchases)
		vendorRoutes.POST("/purchases", vendorHandler.RecordPurchase)
		vendorRoutes.GET("/clients/search", vendorHandler.SearchClients)
		vendorRoutes.POST("/clients/register", vendorHandler.RegisterClient)
		vendorRoutes.GET("/clients/:id/credit-lines", vendorHandler.GetClientCreditLines)
		vendorRoutes.POST("/clients/:id/credit-lines", vendorHandler.RequestCreditLine)
		vendorRoutes.POST("/withdrawals", vendorHandler.RequestWithdrawal)
		vendorRoutes.GET("/withdrawals", vendorHandler.GetMyWithdrawals)
		vendorRoutes.GET("/payments/:id/receipt", vendorHandler.GetMyPaymentReceipt)
	}

	// Admin routes

	// Advisors serve ranchers at the counter: they look a rancher up and quote
	// the quota, but cannot touch the parameters that produce it, approve
	// credit, move money or manage access. Administrators can do both.
	advisorRoutes := authenticated.Group("/admin")
	advisorRoutes.Use(middleware.RequireRole(userRepo, model.RoleAdmin, model.RoleAdvisor))
	{
		advisorRoutes.GET("/clients", clientHandler.ListClients)
		advisorRoutes.GET("/clients/search", clientHandler.ListClients)
		advisorRoutes.GET("/clients/:id", clientHandler.GetClient)
		advisorRoutes.GET("/clients/:id/loans", clientHandler.GetClientLoans)
		advisorRoutes.GET("/clients/:id/credit-lines", clientHandler.GetClientCreditLines)
		advisorRoutes.GET("/clients/:id/payments", clientHandler.GetClientPayments)
		advisorRoutes.GET("/clients/:id/purchases", clientHandler.GetClientPurchases)
		advisorRoutes.GET("/clients/:id/account", clientHandler.GetClientAccount)
		advisorRoutes.GET("/clients/:id/account/movements", clientHandler.GetClientMovements)
		advisorRoutes.GET("/livestock-quotas/:id", livestockQuotaHandler.AssessFromRegistry)
	}

	adminRoutes := authenticated.Group("/admin")
	adminRoutes.Use(middleware.RequireRole(userRepo, model.RoleAdmin))
	{
		adminRoutes.POST("/clients", clientHandler.AdminRegisterClient)
		adminRoutes.PUT("/clients/:id", clientHandler.AdminUpdateClient)
		adminRoutes.PUT("/clients/:id/iva-rate", clientHandler.UpdateIVARate)
		adminRoutes.PUT("/clients/:id/comments", clientHandler.UpdateComments)
		adminRoutes.POST("/clients/:id/block", clientHandler.BlockClient)
		adminRoutes.POST("/clients/:id/unblock", clientHandler.UnblockClient)

		adminRoutes.POST("/credit-lines", creditHandler.CreateCreditLine)
		adminRoutes.GET("/credit-lines/pending", creditHandler.GetPendingCreditLines)
		adminRoutes.PUT("/credit-lines/:id", creditHandler.UpdateCreditLine)
		adminRoutes.POST("/credit-lines/:id/approve", creditHandler.ApproveCreditLine)
		adminRoutes.POST("/credit-lines/:id/reject", creditHandler.RejectCreditLine)

		adminRoutes.POST("/loans", loanHandler.AdminCreateLoan)
		adminRoutes.POST("/loans/withdrawal", loanHandler.AdminCreateWithdrawal)
		adminRoutes.GET("/loans/pending", loanHandler.GetPendingLoans)
		adminRoutes.POST("/loans/:id/approve", loanHandler.ApproveLoan)
		adminRoutes.POST("/loans/:id/disburse", loanHandler.DisburseLoan)
		adminRoutes.POST("/loans/:id/cancel", loanHandler.CancelLoan)
		adminRoutes.POST("/loans/:id/prepay", loanHandler.PrepayLoan)
		adminRoutes.POST("/loans/:id/payments", loanHandler.AdminRecordPayment)
		adminRoutes.GET("/loans/:id/payments/:paymentId/receipt", loanHandler.GetPaymentReceipt)
		adminRoutes.GET("/loans/:id/schedule-pdf", loanHandler.DownloadLoanSchedule)
		adminRoutes.GET("/loans/:id/simulate-cancellation", loanHandler.SimulateCancellation)

		adminRoutes.GET("/dashboard/portfolio", dashboardHandler.GetPortfolio)
		adminRoutes.GET("/dashboard/delinquency", dashboardHandler.GetDelinquency)
		adminRoutes.GET("/dashboard/kpis", dashboardHandler.GetKPIs)
		adminRoutes.GET("/dashboard/trends/disbursements", dashboardHandler.GetDisbursementTrend)
		adminRoutes.GET("/dashboard/trends/collections", dashboardHandler.GetCollectionTrend)

		adminRoutes.GET("/collections/overdue", dashboardHandler.GetOverdueInstallments)

		adminRoutes.GET("/reports/financial", reportHandler.GetFinancialReport)
		adminRoutes.GET("/reports/portfolio", reportHandler.GetPortfolioPosition)

		// Credit risk: applicant data, analysis and availability
		adminRoutes.GET("/risk-profiles", riskHandler.AdminListRiskProfiles)
		adminRoutes.PUT("/risk-profiles", riskHandler.AdminSubmitRiskProfile)
		adminRoutes.GET("/clients/:id/risk-profile", riskHandler.AdminGetRiskProfile)
		adminRoutes.GET("/clients/:id/risk-assessments", riskHandler.GetClientRiskAssessments)
		adminRoutes.POST("/clients/:id/risk-assessments", riskHandler.EvaluateClientRisk)
		adminRoutes.POST("/clients/:id/risk-assessments/apply", riskHandler.ApplyRiskRecommendation)
		adminRoutes.GET("/clients/:id/credit-availability", riskHandler.GetClientCreditAvailability)
		adminRoutes.GET("/risk-assessments/pending", riskHandler.GetPendingRiskReviews)

		// Credit risk: the analysis model itself (prompt + policy guardrails)
		adminRoutes.GET("/risk-models", riskHandler.ListRiskModels)
		adminRoutes.POST("/risk-models", riskHandler.CreateRiskModel)
		adminRoutes.GET("/risk-models/active", riskHandler.GetActiveRiskModel)
		adminRoutes.GET("/risk-models/analyzer", riskHandler.GetRiskAnalyzerInfo)
		adminRoutes.POST("/risk-models/preview", riskHandler.PreviewRiskPrompt)
		adminRoutes.GET("/risk-models/:id", riskHandler.GetRiskModel)
		adminRoutes.PUT("/risk-models/:id", riskHandler.UpdateRiskModel)
		adminRoutes.POST("/risk-models/:id/activate", riskHandler.ActivateRiskModel)
		adminRoutes.POST("/risk-models/:id/versions", riskHandler.SaveRiskModelVersion)

		// Livestock quota engine: the parameters it computes quotas with
		adminRoutes.GET("/livestock-quota-policies", livestockPolicyHandler.ListLivestockQuotaPolicies)
		adminRoutes.POST("/livestock-quota-policies", livestockPolicyHandler.CreateLivestockQuotaPolicy)
		adminRoutes.GET("/livestock-quota-policies/active", livestockPolicyHandler.GetActiveLivestockQuotaPolicy)
		adminRoutes.GET("/livestock-quota-policies/:id", livestockPolicyHandler.GetLivestockQuotaPolicy)
		adminRoutes.PUT("/livestock-quota-policies/:id", livestockPolicyHandler.UpdateLivestockQuotaPolicy)
		adminRoutes.POST("/livestock-quota-policies/:id/activate", livestockPolicyHandler.ActivateLivestockQuotaPolicy)
		adminRoutes.POST("/livestock-quota-policies/:id/versions", livestockPolicyHandler.SaveLivestockQuotaPolicyVersion)

		// User administration. Accounts are deactivated, never deleted: they
		// carry loans, payments and audit entries.

		adminRoutes.GET("/users", userHandler.ListUsers)
		adminRoutes.POST("/users", userHandler.CreateUser)
		adminRoutes.PUT("/users/:id/role", userHandler.ChangeUserRole)
		adminRoutes.PUT("/users/:id/active", userHandler.SetUserActive)

		adminRoutes.GET("/audit", auditHandler.GetAuditLogs)

		// Vendor management (admin)
		adminRoutes.GET("/vendors", vendorHandler.AdminListVendors)
		adminRoutes.GET("/vendors/:id", vendorHandler.AdminGetVendor)
		adminRoutes.POST("/vendors", vendorHandler.AdminRegisterVendor)
		adminRoutes.POST("/vendors/:id/activate", vendorHandler.AdminActivateVendor)
		adminRoutes.POST("/vendors/:id/deactivate", vendorHandler.AdminDeactivateVendor)
		adminRoutes.GET("/vendors/:id/purchases", vendorHandler.AdminGetVendorPurchases)
		adminRoutes.POST("/vendors/:id/purchases", vendorHandler.AdminRecordPurchase)
		adminRoutes.GET("/vendors/:id/payments", vendorHandler.AdminGetVendorPayments)
		adminRoutes.POST("/vendors/:id/payments", vendorHandler.AdminRecordVendorPayment)
		adminRoutes.GET("/vendors/:id/withdrawals", vendorHandler.AdminGetVendorWithdrawals)
		adminRoutes.GET("/vendors/:id/payments/:paymentId/receipt", vendorHandler.AdminGetVendorPaymentReceipt)
		adminRoutes.GET("/withdrawals/pending", vendorHandler.AdminGetPendingWithdrawals)
		adminRoutes.POST("/withdrawals/:id/approve", vendorHandler.AdminApproveWithdrawal)
		adminRoutes.POST("/withdrawals/:id/reject", vendorHandler.AdminRejectWithdrawal)
	}

	// Serve frontend SPA if FRONTEND_DIR is set
	if frontendDir := os.Getenv("FRONTEND_DIR"); frontendDir != "" {
		assetsPrefix := basePath + "/assets"
		r.engine.Static(assetsPrefix, filepath.Join(frontendDir, "assets"))
		r.engine.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			// Serve static files if they exist
			if !strings.HasPrefix(path, basePath+"/api/") && !strings.HasPrefix(path, basePath+"/swagger/") && !strings.HasPrefix(path, "/health") {
				// Strip base path to find the file on disk
				relPath := strings.TrimPrefix(path, basePath)
				if relPath == "" {
					relPath = "/"
				}
				filePath := filepath.Join(frontendDir, relPath)
				if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
					c.File(filePath)
					return
				}
				// SPA fallback: serve index.html
				c.File(filepath.Join(frontendDir, "index.html"))
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})
	}
}
