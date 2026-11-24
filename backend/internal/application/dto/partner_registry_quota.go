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
// rancher, and the e-mail for the promissory note and the electronic invoice of
// ranchers it has none for (most write in by WhatsApp). The phone is for the
// promissory note too: with WhatsApp usernames, Meta may not tell the partner
// the number the rancher writes from. It is the one FEDEGÁN has on file, not a
// verified one: it does not tell who is writing.
type PartnerRancherResponse struct {
	NationalID string                    `json:"nationalId"`
	Name       string                    `json:"name"`
	Email      string                    `json:"email,omitempty"`
	Phone      string                    `json:"phone,omitempty"`
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
			Email:      r.Rancher.Email,
			Phone:      r.Rancher.Phone,
			Holdings:   toRegistryHoldings(r.Rancher.Holdings),
		},
		Assessment:   ToLivestockQuotaResponse(r.Record, r.Assessment, r.Tier),
		HoldingCodes: codes,
		Assumptions:  r.Assumptions,
	}
}
