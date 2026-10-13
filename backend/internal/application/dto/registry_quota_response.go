package dto

import (
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
)

// RegistryQuotaLookupRequest asks for a rancher's quota from the registry.
//
// The national ID travels in the body rather than the path: a URL ends up in
// request logs, proxies and browser history, and a national ID tied to a
// credit lookup is personal data that belongs in none of them.
type RegistryQuotaLookupRequest struct {
	NationalID string `json:"nationalId" binding:"required,max=30"`
	// IDType is the RUV's document type, e.g. "CÉDULA DE CIUDADANÍA" (default).
	IDType string `json:"idType" binding:"max=60"`
}

// RegistryHoldingResponse is one predio as the registry reports it.
type RegistryHoldingResponse struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Department    string `json:"department"`
	Municipality  string `json:"municipality"`
	Village       string `json:"village,omitempty"`
	Cycle         int    `json:"cycle"`
	Year          int    `json:"year"`
	VaccinatedAt  string `json:"vaccinatedAt,omitempty"`
	Head          int    `json:"head"`
	NotVaccinated int    `json:"notVaccinated"`
}

// RegistryRancherResponse is who the registry says the national ID belongs to.
type RegistryRancherResponse struct {
	NationalID string                    `json:"nationalId"`
	Name       string                    `json:"name"`
	Phone      string                    `json:"phone,omitempty"`
	Email      string                    `json:"email,omitempty"`
	Holdings   []RegistryHoldingResponse `json:"holdings"`
}

// RegistryQuotaResponse is a quota computed from the registry.
//
// Assumptions is not decoration: three of the score's inputs are absent from
// the registry, so whoever reads the quota needs to know which figures were
// observed and which were assumed.
type RegistryQuotaResponse struct {
	Rancher     RegistryRancherResponse `json:"rancher"`
	Assessment  LivestockQuotaResponse  `json:"assessment"`
	Assumptions []string                `json:"assumptions"`
}

// ToRegistryQuotaResponse maps a registry-backed assessment.
func ToRegistryQuotaResponse(r *service.RegistryAssessment) RegistryQuotaResponse {
	holdings := make([]RegistryHoldingResponse, 0, len(r.Rancher.Holdings))
	for _, h := range r.Rancher.Holdings {
		c := h.Herd.Classify()
		holdings = append(holdings, RegistryHoldingResponse{
			Code:          h.Code,
			Name:          h.Name,
			Department:    h.Department,
			Municipality:  h.Municipality,
			Village:       h.Village,
			Cycle:         h.Cycle,
			Year:          h.Year,
			VaccinatedAt:  h.VaccinatedAt,
			Head:          c.Total,
			NotVaccinated: c.NotVaccinated,
		})
	}

	return RegistryQuotaResponse{
		Rancher: RegistryRancherResponse{
			NationalID: r.Rancher.NationalID,
			Name:       r.Rancher.Name,
			Phone:      r.Rancher.Phone,
			Email:      r.Rancher.Email,
			Holdings:   holdings,
		},
		Assessment:  ToLivestockQuotaResponse(r.Record, r.Assessment),
		Assumptions: r.Assumptions,
	}
}
