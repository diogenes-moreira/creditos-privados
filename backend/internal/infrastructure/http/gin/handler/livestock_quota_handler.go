package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// LivestockQuotaHandler exposes the RUV-based livestock quota engine to partner
// services (CuppoGan), authenticated by service key rather than by user.
type LivestockQuotaHandler struct {
	service *service.LivestockQuotaService
}

func NewLivestockQuotaHandler(s *service.LivestockQuotaService) *LivestockQuotaHandler {
	return &LivestockQuotaHandler{service: s}
}

// Assess godoc
// @Summary Assess a rancher's livestock credit quota
// @Description Runs the RUV-based livestock quota engine for a rancher and stores the result. The holdings are summed: the quota is one per national ID.
// @Tags Livestock quota
// @Accept json
// @Produce json
// @Param request body dto.LivestockQuotaRequest true "Rancher's RUV data"
// @Success 200 {object} dto.LivestockQuotaResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Security ServiceKey
// @Router /livestock-quotas [post]
func (h *LivestockQuotaHandler) Assess(c *gin.Context) {
	var req dto.LivestockQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	record, assessment, err := h.service.Assess(c.Request.Context(), dto.ToLivestockQuotaRequest(req), c.GetString("serviceName"))
	if errors.Is(err, service.ErrInvalidLivestockQuotaRequest) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockQuotaResponse(record, assessment))
}

// AssessFromRegistry godoc
// @Summary Quote a rancher's credit from the RUV
// @Description Reads the national ID's herds from Fedegán's vaccination registry and returns the quota with the full trace of how it was reached
// @Tags Admin Livestock Quota
// @Produce json
// @Param id path string true "National ID"
// @Param idType query string false "CÉDULA DE CIUDADANÍA (default), NIT, PASAPORTE..."
// @Success 200 {object} dto.RegistryQuotaResponse
// @Failure 404 {object} dto.ErrorResponse "Not in the registry"
// @Failure 503 {object} dto.ErrorResponse "Registry not configured"
// @Security BearerAuth
// @Router /admin/livestock-quotas/{id} [get]
func (h *LivestockQuotaHandler) AssessFromRegistry(c *gin.Context) {
	nationalID := strings.TrimSpace(c.Param("id"))
	if nationalID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "national id is required"})
		return
	}

	actor, _ := c.Get("email")
	requestedBy, _ := actor.(string)
	if requestedBy == "" {
		requestedBy = "admin"
	}

	result, err := h.service.AssessFromRegistry(c.Request.Context(), c.Query("idType"), nationalID, requestedBy)
	switch {
	case errors.Is(err, service.ErrNotInRegistry):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "no RUV record for that national ID"})
		return
	case errors.Is(err, service.ErrRegistryUnavailable):
		c.JSON(http.StatusServiceUnavailable, dto.ErrorResponse{Error: "the RUV is not configured"})
		return
	case errors.Is(err, service.ErrInvalidLivestockQuotaRequest):
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	case err != nil:
		c.JSON(http.StatusBadGateway, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.ToRegistryQuotaResponse(result))
}
