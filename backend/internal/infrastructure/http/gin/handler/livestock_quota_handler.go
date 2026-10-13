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

// AssessFromRegistryForPartner godoc
// @Summary Assess a rancher's livestock quota from the RUV (partner)
// @Description Reads the national ID's herds from Fedegán's vaccination registry, runs the quota engine and stores the result. The partner sends only the identity; contact data is not returned.
// @Tags Livestock quota
// @Accept json
// @Produce json
// @Param request body dto.PartnerRegistryQuotaRequest true "Rancher's identity"
// @Success 200 {object} dto.PartnerRegistryQuotaResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse "Not in the registry"
// @Failure 502 {object} dto.ErrorResponse "Registry error"
// @Failure 503 {object} dto.ErrorResponse "Registry not configured"
// @Security ServiceKey
// @Router /livestock-quotas/registry [post]
func (h *LivestockQuotaHandler) AssessFromRegistryForPartner(c *gin.Context) {
	var req dto.PartnerRegistryQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	result, err := h.service.AssessFromRegistry(c.Request.Context(), req.IDType, strings.TrimSpace(req.NationalID),
		c.GetString("serviceName"), req.Reference)
	if status, ok := registryErrorStatus(err); ok {
		c.JSON(status, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToPartnerRegistryQuotaResponse(result))
}

// registryErrorStatus maps a registry assessment error to its HTTP status.
func registryErrorStatus(err error) (int, bool) {
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, service.ErrNotInRegistry):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrRegistryUnavailable):
		return http.StatusServiceUnavailable, true
	case errors.Is(err, service.ErrInvalidLivestockQuotaRequest):
		return http.StatusBadRequest, true
	default:
		return http.StatusBadGateway, true
	}
}

// AssessFromRegistry godoc
// @Summary Quote a rancher's credit from the RUV
// @Description Reads the national ID's herds from Fedegán's vaccination registry, stores the assessment as the record of who quoted what, and returns the quota with the full trace of how it was reached
// @Tags Admin Livestock Quota
// @Accept json
// @Produce json
// @Param request body dto.RegistryQuotaLookupRequest true "Rancher's identity"
// @Success 200 {object} dto.RegistryQuotaResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse "Not in the registry"
// @Failure 503 {object} dto.ErrorResponse "Registry not configured"
// @Security BearerAuth
// @Router /admin/livestock-quotas/registry [post]
//
// It is a POST because every call queries the RUV and stores an assessment: a
// GET would be repeated by anything that treats reads as safe to replay.
func (h *LivestockQuotaHandler) AssessFromRegistry(c *gin.Context) {
	var req dto.RegistryQuotaLookupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	nationalID := strings.TrimSpace(req.NationalID)
	if nationalID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "national id is required"})
		return
	}

	actor, _ := c.Get("email")
	requestedBy, _ := actor.(string)
	if requestedBy == "" {
		requestedBy = "admin"
	}

	result, err := h.service.AssessFromRegistry(c.Request.Context(), req.IDType, nationalID, requestedBy, "")
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
