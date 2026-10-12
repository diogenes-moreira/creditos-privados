package dto

import (
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// LivestockQuotaCeilingsResponse are the four ceilings; the quota is the lowest.
type LivestockQuotaCeilingsResponse struct {
	LTV       string `json:"ltv"`
	Capacity  string `json:"capacity"`
	Inputs    string `json:"inputs"`
	PolicyCap string `json:"policyCap"`
}

// LivestockScoreResponse is the 0-100 behaviour score and its components.
type LivestockScoreResponse struct {
	Score              int    `json:"score"`
	Grade              string `json:"grade"`
	VaccinationRatePct int    `json:"vaccinationRatePct"`
	CycleCompliancePct int    `json:"cycleCompliancePct"`
	SeniorityCycles    int    `json:"seniorityCycles"`
}

// LivestockQuotaResponse is the assigned quota with the trace of how it was reached.
type LivestockQuotaResponse struct {
	ID            string                         `json:"id"`
	NationalID    string                         `json:"nationalId"`
	Reference     string                         `json:"reference"`
	Quota         string                         `json:"quota"`
	Currency      string                         `json:"currency"`
	ActiveCeiling string                         `json:"activeCeiling"`
	Ceilings      LivestockQuotaCeilingsResponse `json:"ceilings"`
	TermMonths    int                            `json:"termMonths"`
	PeriodRatePct string                         `json:"periodRatePct"`
	Score         LivestockScoreResponse         `json:"score"`
	Typology      string                         `json:"typology"`
	Head          int                            `json:"head"`
	UGG           string                         `json:"ugg"`
	// ProjectedIncome is the holding's expected revenue over the term.
	ProjectedIncome string `json:"projectedIncome"`
	// EligibleBasket is what the rancher would buy in the inputs catalogue over the term.
	EligibleBasket string    `json:"eligibleBasket"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ToLivestockQuotaRequest maps the HTTP request to the service input.
func ToLivestockQuotaRequest(r LivestockQuotaRequest) service.LivestockQuotaRequest {
	holdings := make([]model.Herd, 0, len(r.Holdings))
	for _, h := range r.Holdings {
		holdings = append(holdings, model.Herd{
			FemalesUnder3Months: h.FemalesUnder3Months,
			Females3To8Months:   h.Females3To8Months,
			Females8To12Months:  h.Females8To12Months,
			Females1To2Years:    h.Females1To2Years,
			Females2To3Years:    h.Females2To3Years,
			Females3To5Years:    h.Females3To5Years,
			FemalesOver5Years:   h.FemalesOver5Years,
			MalesUnder3Months:   h.MalesUnder3Months,
			Males3To8Months:     h.Males3To8Months,
			Males8To12Months:    h.Males8To12Months,
			Males1To2Years:      h.Males1To2Years,
			Males2To3Years:      h.Males2To3Years,
			MalesOver3Years:     h.MalesOver3Years,
			NotVaccinated:       h.NotVaccinated,
		})
	}
	return service.LivestockQuotaRequest{
		NationalID:      r.NationalID,
		Department:      r.Department,
		Holdings:        holdings,
		Tenure:          r.Tenure,
		HoldingStatus:   r.HoldingStatus,
		SeniorityCycles: r.SeniorityCycles,
		CompletedCycles: r.CompletedCycles,
		ReportedCycles:  r.ReportedCycles,
		Reference:       r.Reference,
	}
}

// ToLivestockQuotaResponse maps a stored assessment and its engine result.
func ToLivestockQuotaResponse(r *model.LivestockQuotaAssessment, a model.LivestockAssessment) LivestockQuotaResponse {
	q := a.Quota
	return LivestockQuotaResponse{
		ID:            r.ID.String(),
		NationalID:    r.NationalID,
		Reference:     r.Reference,
		Quota:         r.Quota.StringFixed(2),
		Currency:      r.Currency,
		ActiveCeiling: q.ActiveCeiling,
		Ceilings: LivestockQuotaCeilingsResponse{
			LTV:       q.LTVCeiling.StringFixed(2),
			Capacity:  q.CapacityCeiling.StringFixed(2),
			Inputs:    q.InputsCeiling.StringFixed(2),
			PolicyCap: q.PolicyCap.StringFixed(2),
		},
		TermMonths:    q.TermMonths,
		PeriodRatePct: q.PeriodRatePct.StringFixed(4),
		Score: LivestockScoreResponse{
			Score:              a.Score.Score,
			Grade:              a.Score.Grade(),
			VaccinationRatePct: a.Score.VaccinationRatePct,
			CycleCompliancePct: a.Score.CycleComplaincePct,
			SeniorityCycles:    a.Score.SeniorityCycles,
		},
		Typology:        string(a.Typology),
		Head:            a.Classification.Total,
		UGG:             a.UGG.StringFixed(2),
		ProjectedIncome: a.Income.Total.StringFixed(2),
		EligibleBasket:  a.Costs.EligibleBasket.StringFixed(2),
		CreatedAt:       r.CreatedAt,
	}
}
