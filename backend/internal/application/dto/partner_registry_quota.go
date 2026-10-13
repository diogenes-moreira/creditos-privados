package dto

import "github.com/diogenes-moreira/creditos/backend/internal/application/service"

// PartnerRegistryQuotaRequest asks for a rancher's quota from the RUV. The
// partner sends only the identity; Prestia reads the herds itself.
type PartnerRegistryQuotaRequest struct {
	NationalID string `json:"nationalId" binding:"required,max=30"`
	// IDType is the RUV's document type, e.g. "CÉDULA DE CIUDADANÍA" (default).
	IDType string `json:"idType" binding:"max=60"`
	// Reference is the caller's identifier for the request (e.g. its onboarding ID).
	Reference string `json:"reference" binding:"max=100"`
}

// PartnerRegistryQuotaResponse is the quota a partner gets. It leaves out the
// rancher's contact data, which the partner already holds, and keeps what it
// needs to explain the figure: the RUV codes summed and what was assumed.
type PartnerRegistryQuotaResponse struct {
	Assessment   LivestockQuotaResponse `json:"assessment"`
	HoldingCodes []string               `json:"holdingCodes"`
	Assumptions  []string               `json:"assumptions"`
}

// ToPartnerRegistryQuotaResponse maps a registry-backed assessment for a partner.
func ToPartnerRegistryQuotaResponse(r *service.RegistryAssessment) PartnerRegistryQuotaResponse {
	codes := []string{}
	for _, h := range r.Rancher.LatestHoldings() {
		codes = append(codes, h.Code)
	}
	return PartnerRegistryQuotaResponse{
		Assessment:   ToLivestockQuotaResponse(r.Record, r.Assessment),
		HoldingCodes: codes,
		Assumptions:  r.Assumptions,
	}
}
