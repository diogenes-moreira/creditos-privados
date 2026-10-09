package handler

import (
	"net/http"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RiskHandler exposes the credit-risk module over the REST API: risk-data
// ingestion, analysis, credit availability, and management of the analysis
// model itself.
type RiskHandler struct {
	riskService *service.RiskService
	clientRepo  port.ClientRepository
}

func NewRiskHandler(riskService *service.RiskService, clientRepo port.ClientRepository) *RiskHandler {
	return &RiskHandler{riskService: riskService, clientRepo: clientRepo}
}

// resolveClient maps the authenticated user to their client record.
func (h *RiskHandler) resolveClient(c *gin.Context) (*model.Client, bool) {
	userID := c.MustGet("userID").(uuid.UUID)
	client, err := h.clientRepo.FindByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "client profile not found"})
		return nil, false
	}
	return client, true
}

func actorID(c *gin.Context) *uuid.UUID {
	if raw, ok := c.Get("userID"); ok {
		if id, ok := raw.(uuid.UUID); ok {
			return &id
		}
	}
	return nil
}

// ==================== client self-service ====================

// SubmitMyRiskProfile godoc
// @Summary Submit my credit-risk data
// @Description Creates or replaces the authenticated client's credit-risk data, the input to the credit availability analysis
// @Tags Risk
// @Accept json
// @Produce json
// @Param request body dto.SubmitRiskProfileRequest true "Risk data"
// @Success 200 {object} dto.RiskProfileResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /me/risk-profile [put]
func (h *RiskHandler) SubmitMyRiskProfile(c *gin.Context) {
	client, ok := h.resolveClient(c)
	if !ok {
		return
	}
	var req dto.SubmitRiskProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	in, err := dto.ToRiskProfileInput(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	profile, err := h.riskService.SubmitProfile(c.Request.Context(), actorID(c), client.ID, in, model.RiskSourceClient)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskProfileResponse(profile, client.FullName()))
}

// GetMyRiskProfile godoc
// @Summary Get my credit-risk data
// @Tags Risk
// @Produce json
// @Success 200 {object} dto.RiskProfileResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /me/risk-profile [get]
func (h *RiskHandler) GetMyRiskProfile(c *gin.Context) {
	client, ok := h.resolveClient(c)
	if !ok {
		return
	}
	profile, err := h.riskService.GetProfile(c.Request.Context(), client.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "no risk data on file"})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskProfileResponse(profile, client.FullName()))
}

// GetMyCreditAvailability godoc
// @Summary Get my available credit
// @Description Returns the credit limit the risk analysis recommends and how much of it is still unused
// @Tags Risk
// @Produce json
// @Success 200 {object} dto.CreditAvailabilityResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /me/credit-availability [get]
func (h *RiskHandler) GetMyCreditAvailability(c *gin.Context) {
	client, ok := h.resolveClient(c)
	if !ok {
		return
	}
	availability, err := h.riskService.AvailableCredit(c.Request.Context(), client.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToCreditAvailabilityResponse(availability))
}

// SimulateRisk godoc
// @Summary Pre-qualify without an account
// @Description Scores hypothetical risk data and returns the credit it would support. Nothing is stored.
// @Tags Risk
// @Accept json
// @Produce json
// @Param request body dto.SimulateRiskRequest true "Hypothetical risk data"
// @Success 200 {object} dto.RiskEvaluationResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /risk/simulate [post]
func (h *RiskHandler) SimulateRisk(c *gin.Context) {
	var req dto.SimulateRiskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	in, err := dto.ToRiskProfileInput(req.SubmitRiskProfileRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	eval, err := h.riskService.Simulate(c.Request.Context(), in, req.Age)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskEvaluationResponse(eval))
}

// ==================== admin: risk data and analysis ====================

// AdminSubmitRiskProfile godoc
// @Summary Submit a client's credit-risk data
// @Tags Risk
// @Accept json
// @Produce json
// @Param request body dto.AdminSubmitRiskProfileRequest true "Risk data"
// @Success 200 {object} dto.RiskProfileResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-profiles [put]
func (h *RiskHandler) AdminSubmitRiskProfile(c *gin.Context) {
	var req dto.AdminSubmitRiskProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	clientID, err := uuid.Parse(req.ClientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	in, err := dto.ToRiskProfileInput(req.SubmitRiskProfileRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	profile, err := h.riskService.SubmitProfile(c.Request.Context(), actorID(c), clientID, in, model.RiskSourceAdmin)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskProfileResponse(profile, ""))
}

// AdminGetRiskProfile godoc
// @Summary Get a client's credit-risk data
// @Tags Risk
// @Produce json
// @Param id path string true "Client UUID"
// @Success 200 {object} dto.RiskProfileResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/clients/{id}/risk-profile [get]
func (h *RiskHandler) AdminGetRiskProfile(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	profile, err := h.riskService.GetProfile(c.Request.Context(), clientID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "no risk data on file"})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskProfileResponse(profile, ""))
}

// AdminListRiskProfiles godoc
// @Summary List submitted credit-risk data
// @Tags Risk
// @Produce json
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} dto.PaginatedResponse
// @Security BearerAuth
// @Router /admin/risk-profiles [get]
func (h *RiskHandler) AdminListRiskProfiles(c *gin.Context) {
	var req dto.PaginationRequest
	_ = c.ShouldBindQuery(&req)
	if req.Limit <= 0 {
		req.Limit = 20
	}
	profiles, total, err := h.riskService.ListProfiles(c.Request.Context(), req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data: dto.ToRiskProfileResponses(profiles), Total: total, Offset: req.Offset, Limit: req.Limit,
	})
}

// EvaluateClientRisk godoc
// @Summary Run the risk analysis for a client
// @Description Runs the active analysis model over the client's risk data and stores the assessment
// @Tags Risk
// @Produce json
// @Param id path string true "Client UUID"
// @Success 201 {object} dto.RiskAssessmentResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/clients/{id}/risk-assessments [post]
func (h *RiskHandler) EvaluateClientRisk(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	assessment, err := h.riskService.Evaluate(c.Request.Context(), actorID(c), clientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ToRiskAssessmentResponse(assessment))
}

// GetClientRiskAssessments godoc
// @Summary List a client's risk assessments
// @Tags Risk
// @Produce json
// @Param id path string true "Client UUID"
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} dto.PaginatedResponse
// @Security BearerAuth
// @Router /admin/clients/{id}/risk-assessments [get]
func (h *RiskHandler) GetClientRiskAssessments(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	var req dto.PaginationRequest
	_ = c.ShouldBindQuery(&req)
	if req.Limit <= 0 {
		req.Limit = 20
	}
	assessments, total, err := h.riskService.ListAssessments(c.Request.Context(), clientID, req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data: dto.ToRiskAssessmentResponses(assessments), Total: total, Offset: req.Offset, Limit: req.Limit,
	})
}

// GetClientCreditAvailability godoc
// @Summary Get a client's available credit
// @Tags Risk
// @Produce json
// @Param id path string true "Client UUID"
// @Success 200 {object} dto.CreditAvailabilityResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/clients/{id}/credit-availability [get]
func (h *RiskHandler) GetClientCreditAvailability(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	availability, err := h.riskService.AvailableCredit(c.Request.Context(), clientID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToCreditAvailabilityResponse(availability))
}

// ApplyRiskRecommendation godoc
// @Summary Apply the recommended limit to the client's credit line
// @Description Creates or updates the credit line from the latest approved assessment
// @Tags Risk
// @Produce json
// @Param id path string true "Client UUID"
// @Success 200 {object} dto.CreditLineResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/clients/{id}/risk-assessments/apply [post]
func (h *RiskHandler) ApplyRiskRecommendation(c *gin.Context) {
	adminID := c.MustGet("userID").(uuid.UUID)
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
		return
	}
	cl, err := h.riskService.ApplyRecommendation(c.Request.Context(), adminID, clientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToCreditLineResponse(cl))
}

// GetPendingRiskReviews godoc
// @Summary List assessments awaiting a manual decision
// @Tags Risk
// @Produce json
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} dto.PaginatedResponse
// @Security BearerAuth
// @Router /admin/risk-assessments/pending [get]
func (h *RiskHandler) GetPendingRiskReviews(c *gin.Context) {
	var req dto.PaginationRequest
	_ = c.ShouldBindQuery(&req)
	if req.Limit <= 0 {
		req.Limit = 20
	}
	assessments, total, err := h.riskService.PendingReviews(c.Request.Context(), req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data: dto.ToRiskAssessmentResponses(assessments), Total: total, Offset: req.Offset, Limit: req.Limit,
	})
}

// ==================== admin: analysis model ====================

// GetActiveRiskModel godoc
// @Summary Get the active analysis model
// @Description Returns the analysis prompt and policy guardrails currently in use
// @Tags Risk Models
// @Produce json
// @Success 200 {object} dto.RiskModelResponse
// @Failure 500 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/active [get]
func (h *RiskHandler) GetActiveRiskModel(c *gin.Context) {
	m, err := h.riskService.ActiveModel(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskModelResponse(m))
}

// ListRiskModels godoc
// @Summary List analysis model versions
// @Tags Risk Models
// @Produce json
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} dto.PaginatedResponse
// @Security BearerAuth
// @Router /admin/risk-models [get]
func (h *RiskHandler) ListRiskModels(c *gin.Context) {
	var req dto.PaginationRequest
	_ = c.ShouldBindQuery(&req)
	if req.Limit <= 0 {
		req.Limit = 20
	}
	models, total, err := h.riskService.ListModels(c.Request.Context(), req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data: dto.ToRiskModelResponses(models), Total: total, Offset: req.Offset, Limit: req.Limit,
	})
}

// GetRiskModel godoc
// @Summary Get one analysis model version
// @Tags Risk Models
// @Produce json
// @Param id path string true "Risk Model UUID"
// @Success 200 {object} dto.RiskModelResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/{id} [get]
func (h *RiskHandler) GetRiskModel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid risk model ID"})
		return
	}
	m, err := h.riskService.GetModel(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskModelResponse(m))
}

// CreateRiskModel godoc
// @Summary Create an analysis model
// @Description Registers a new analysis prompt and its policy guardrails
// @Tags Risk Models
// @Accept json
// @Produce json
// @Param request body dto.CreateRiskModelRequest true "Analysis model"
// @Success 201 {object} dto.RiskModelResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models [post]
func (h *RiskHandler) CreateRiskModel(c *gin.Context) {
	var req dto.CreateRiskModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToRiskModelParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	bands, err := dto.ToRiskGradeBands(req.Bands)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	m, err := h.riskService.CreateModel(c.Request.Context(), actorID(c), req.Name, req.Prompt, req.Description, params, bands)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	if req.Activate {
		if m, err = h.riskService.ActivateModel(c.Request.Context(), actorID(c), m.ID); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
	}
	c.JSON(http.StatusCreated, dto.ToRiskModelResponse(m))
}

// SaveRiskModelVersion godoc
// @Summary Save a new version of an analysis model
// @Description Derives a new version from an existing model. The running version keeps serving until the new one is activated.
// @Tags Risk Models
// @Accept json
// @Produce json
// @Param id path string true "Source Risk Model UUID"
// @Param request body dto.SaveRiskModelVersionRequest true "New version"
// @Success 201 {object} dto.RiskModelResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/{id}/versions [post]
func (h *RiskHandler) SaveRiskModelVersion(c *gin.Context) {
	sourceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid risk model ID"})
		return
	}
	var req dto.SaveRiskModelVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToRiskModelParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	bands, err := dto.ToRiskGradeBands(req.Bands)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	m, err := h.riskService.SaveModelVersion(c.Request.Context(), actorID(c), sourceID,
		req.Prompt, req.Description, params, bands, req.Activate)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ToRiskModelResponse(m))
}

// UpdateRiskModel godoc
// @Summary Update an analysis model version in place
// @Tags Risk Models
// @Accept json
// @Produce json
// @Param id path string true "Risk Model UUID"
// @Param request body dto.UpdateRiskModelRequest true "Model changes"
// @Success 200 {object} dto.RiskModelResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/{id} [put]
func (h *RiskHandler) UpdateRiskModel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid risk model ID"})
		return
	}
	var req dto.UpdateRiskModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToRiskModelParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	bands, err := dto.ToRiskGradeBands(req.Bands)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	m, err := h.riskService.UpdateModel(c.Request.Context(), actorID(c), id, req.Prompt, req.Description, params, bands)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskModelResponse(m))
}

// ActivateRiskModel godoc
// @Summary Activate an analysis model version
// @Description Makes this version the one used for new evaluations
// @Tags Risk Models
// @Produce json
// @Param id path string true "Risk Model UUID"
// @Success 200 {object} dto.RiskModelResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/{id}/activate [post]
func (h *RiskHandler) ActivateRiskModel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid risk model ID"})
		return
	}
	m, err := h.riskService.ActivateModel(c.Request.Context(), actorID(c), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToRiskModelResponse(m))
}

// PreviewRiskPrompt godoc
// @Summary Preview an analysis prompt
// @Description Renders a candidate prompt against a client's real data, or sample data, without saving or sending anything
// @Tags Risk Models
// @Accept json
// @Produce json
// @Param request body dto.PreviewPromptRequest true "Candidate prompt"
// @Success 200 {object} dto.PromptPreviewResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/risk-models/preview [post]
func (h *RiskHandler) PreviewRiskPrompt(c *gin.Context) {
	var req dto.PreviewPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	var clientID *uuid.UUID
	if req.ClientID != "" {
		id, err := uuid.Parse(req.ClientID)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client ID"})
			return
		}
		clientID = &id
	}
	preview, err := h.riskService.PreviewPrompt(c.Request.Context(), req.Prompt, clientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToPromptPreviewResponse(preview))
}

// GetRiskAnalyzerInfo godoc
// @Summary Describe the configured analysis engine
// @Tags Risk Models
// @Produce json
// @Success 200 {object} dto.RiskAnalyzerInfoResponse
// @Security BearerAuth
// @Router /admin/risk-models/analyzer [get]
func (h *RiskHandler) GetRiskAnalyzerInfo(c *gin.Context) {
	name := h.riskService.AnalyzerName()
	c.JSON(http.StatusOK, dto.RiskAnalyzerInfoResponse{
		Analyzer:     name,
		PromptDriven: name != model.AnalyzerScorecard,
		Placeholders: model.KnownPromptPlaceholders,
	})
}
