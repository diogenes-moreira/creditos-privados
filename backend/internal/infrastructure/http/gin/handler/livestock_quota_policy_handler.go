package handler

import (
	"net/http"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// LivestockQuotaPolicyHandler exposes the livestock quota engine's parameters
// to administrators: prices, costs, labour thresholds, credit policy and the
// UGG conversion matrix, versioned and activated one at a time.
type LivestockQuotaPolicyHandler struct {
	policies *service.LivestockQuotaPolicyService
}

func NewLivestockQuotaPolicyHandler(policies *service.LivestockQuotaPolicyService) *LivestockQuotaPolicyHandler {
	return &LivestockQuotaPolicyHandler{policies: policies}
}

// GetActiveLivestockQuotaPolicy godoc
// @Summary Get the active livestock quota parameters
// @Description Returns the parameter set every new quota is computed with, seeding the shipped defaults the first time it is asked for
// @Tags Livestock quota policies
// @Produce json
// @Success 200 {object} dto.LivestockQuotaPolicyResponse
// @Failure 500 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies/active [get]
func (h *LivestockQuotaPolicyHandler) GetActiveLivestockQuotaPolicy(c *gin.Context) {
	p, err := h.policies.EnsureDefaultPolicy(c.Request.Context(), actorID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockQuotaPolicyResponse(p))
}

// ListLivestockQuotaPolicies godoc
// @Summary List livestock quota parameter versions
// @Tags Livestock quota policies
// @Produce json
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} dto.PaginatedResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies [get]
func (h *LivestockQuotaPolicyHandler) ListLivestockQuotaPolicies(c *gin.Context) {
	var req dto.PaginationRequest
	_ = c.ShouldBindQuery(&req)
	if req.Limit <= 0 {
		req.Limit = 20
	}
	policies, total, err := h.policies.List(c.Request.Context(), req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data: dto.ToLivestockQuotaPolicyResponses(policies), Total: total, Offset: req.Offset, Limit: req.Limit,
	})
}

// GetLivestockQuotaPolicy godoc
// @Summary Get one livestock quota parameter version
// @Tags Livestock quota policies
// @Produce json
// @Param id path string true "Policy UUID"
// @Success 200 {object} dto.LivestockQuotaPolicyResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies/{id} [get]
func (h *LivestockQuotaPolicyHandler) GetLivestockQuotaPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid livestock quota policy ID"})
		return
	}
	p, err := h.policies.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockQuotaPolicyResponse(p))
}

// CreateLivestockQuotaPolicy godoc
// @Summary Create a livestock quota parameter set
// @Tags Livestock quota policies
// @Accept json
// @Produce json
// @Param request body dto.CreateLivestockQuotaPolicyRequest true "Parameter set"
// @Success 201 {object} dto.LivestockQuotaPolicyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies [post]
func (h *LivestockQuotaPolicyHandler) CreateLivestockQuotaPolicy(c *gin.Context) {
	var req dto.CreateLivestockQuotaPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToLivestockQuotaParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	p, err := h.policies.Create(c.Request.Context(), actorID(c), req.Name, req.Description, params, req.Activate)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ToLivestockQuotaPolicyResponse(p))
}

// SaveLivestockQuotaPolicyVersion godoc
// @Summary Save a new version of a livestock quota parameter set
// @Description Derives a new version from an existing policy. The running version keeps serving until the new one is activated.
// @Tags Livestock quota policies
// @Accept json
// @Produce json
// @Param id path string true "Source policy UUID"
// @Param request body dto.SaveLivestockQuotaPolicyVersionRequest true "New version"
// @Success 201 {object} dto.LivestockQuotaPolicyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies/{id}/versions [post]
func (h *LivestockQuotaPolicyHandler) SaveLivestockQuotaPolicyVersion(c *gin.Context) {
	sourceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid livestock quota policy ID"})
		return
	}
	var req dto.SaveLivestockQuotaPolicyVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToLivestockQuotaParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	p, err := h.policies.SaveVersion(c.Request.Context(), actorID(c), sourceID, req.Description, params, req.Activate)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ToLivestockQuotaPolicyResponse(p))
}

// UpdateLivestockQuotaPolicy godoc
// @Summary Update a livestock quota parameter version in place
// @Tags Livestock quota policies
// @Accept json
// @Produce json
// @Param id path string true "Policy UUID"
// @Param request body dto.UpdateLivestockQuotaPolicyRequest true "Parameter changes"
// @Success 200 {object} dto.LivestockQuotaPolicyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies/{id} [put]
func (h *LivestockQuotaPolicyHandler) UpdateLivestockQuotaPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid livestock quota policy ID"})
		return
	}
	var req dto.UpdateLivestockQuotaPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	params, err := dto.ToLivestockQuotaParams(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	p, err := h.policies.Update(c.Request.Context(), actorID(c), id, req.Description, params)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockQuotaPolicyResponse(p))
}

// ActivateLivestockQuotaPolicy godoc
// @Summary Activate a livestock quota parameter version
// @Description Makes this version the parameters every new quota is computed with
// @Tags Livestock quota policies
// @Produce json
// @Param id path string true "Policy UUID"
// @Success 200 {object} dto.LivestockQuotaPolicyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-quota-policies/{id}/activate [post]
func (h *LivestockQuotaPolicyHandler) ActivateLivestockQuotaPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid livestock quota policy ID"})
		return
	}
	p, err := h.policies.Activate(c.Request.Context(), actorID(c), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockQuotaPolicyResponse(p))
}
