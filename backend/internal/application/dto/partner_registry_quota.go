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

// PartnerRancherResponse is who the RUV says the national ID belongs to, with
// every predio and cycle reported. The partner needs the predios to fill the
// promissory note (notification address) and to cite the RUV code to the
// rancher; the contact data stays out, since the partner already holds it.
type PartnerRancherResponse struct {
	NationalID string                    `json:"nationalId"`
	Name       string                    `json:"name"`
	Holdings   []RegistryHoldingResponse `json:"holdings"`
}

// PartnerRegistryQuotaResponse is the quota a partner gets: the figure, the
// predios behind it (the RUV codes summed) and what was assumed.
type PartnerRegistryQuotaResponse struct {
	Rancher      PartnerRancherResponse `json:"rancher"`
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
		Rancher: PartnerRancherResponse{
			NationalID: r.Rancher.NationalID,
			Name:       r.Rancher.Name,
			Holdings:   toRegistryHoldings(r.Rancher.Holdings),
		},
		Assessment:   ToLivestockQuotaResponse(r.Record, r.Assessment),
		HoldingCodes: codes,
		Assumptions:  r.Assumptions,
	}
}
