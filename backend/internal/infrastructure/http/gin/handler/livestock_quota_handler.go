package handler

import (
	"errors"
	"net/http"

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
