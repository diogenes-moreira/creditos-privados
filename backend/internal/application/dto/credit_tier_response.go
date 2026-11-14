package dto

import (
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// TierCriterionResponse is one criterion of the pre-approval table.
type TierCriterionResponse struct {
	Code string `json:"code"`
	// Status is met, unmet or unverified.
	Status   string `json:"status"`
	Actual   string `json:"actual,omitempty"`
	Required string `json:"required,omitempty"`
}

// TierEvaluationResponse is one layer checked against the registry.
type TierEvaluationResponse struct {
	Code      string                  `json:"code"`
	Amount    string                  `json:"amount"`
	Qualifies bool                    `json:"qualifies"`
	Criteria  []TierCriterionResponse `json:"criteria"`
}

// VaccinationRecordResponse is the registry history the layers are read from.
type VaccinationRecordResponse struct {
	LatestYear      int    `json:"latestYear"`
	LatestCycle     int    `json:"latestCycle"`
	VaccinatedHeads int    `json:"vaccinatedHeads"`
	NotVaccinated   int    `json:"notVaccinated"`
	ReportedCycles  int    `json:"reportedCycles"`
	CompleteCycles  int    `json:"completeCycles"`
	VaccinationRate string `json:"vaccinationRate"`
	CompleteRatio   string `json:"completeRatio"`
	HoldingActive   bool   `json:"holdingActive"`
	// HistoryKnown is false when the registry returned only the current
	// cycle, so the cycle counts above are not a history.
	HistoryKnown bool `json:"historyKnown"`
}

// CreditTierResponse is the pre-approval by layers: the amount available, the
// layer it comes from, and what is still to be verified.
type CreditTierResponse struct {
	// Code is the layer reached (500k, 1M, 2M); empty when none is.
	Code string `json:"code"`
	// Amount is the credit available, as a decimal string; "0" with no layer.
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	// Eligible is false when an entry filter is known to fail.
	Eligible bool `json:"eligible"`
	// Pending lists entry filters nobody has verified yet: the pre-approval
	// holds only once they are confirmed.
	Pending []string `json:"pending"`
	// ProvisionalCode and ProvisionalAmount are the highest layer no known
	// fact rules out: what the rancher reaches once everything unverified,
	// here or in the layers, is confirmed.
	ProvisionalCode   string                    `json:"provisionalCode"`
	ProvisionalAmount string                    `json:"provisionalAmount"`
	Entry             []TierCriterionResponse   `json:"entry"`
	Tiers             []TierEvaluationResponse  `json:"tiers"`
	Vaccination       VaccinationRecordResponse `json:"vaccination"`
}

func toCriteria(cs []model.TierCriterion) []TierCriterionResponse {
	out := make([]TierCriterionResponse, 0, len(cs))
	for _, c := range cs {
		out = append(out, TierCriterionResponse{Code: c.Code, Status: string(c.Status), Actual: c.Actual, Required: c.Required})
	}
	return out
}

// ToCreditTierResponse maps a tier assessment and the history behind it.
func ToCreditTierResponse(t model.TierAssessment, v model.VaccinationRecord, currency string) CreditTierResponse {
	tiers := make([]TierEvaluationResponse, 0, len(t.Tiers))
	for _, e := range t.Tiers {
		tiers = append(tiers, TierEvaluationResponse{Code: e.Code, Amount: e.Amount.StringFixed(2),
			Qualifies: e.Qualifies, Criteria: toCriteria(e.Criteria)})
	}
	pending := t.Pending
	if pending == nil {
		pending = []string{}
	}
	return CreditTierResponse{
		Code:              t.Code,
		Amount:            t.Amount.StringFixed(2),
		Currency:          currency,
		Eligible:          t.Eligible,
		Pending:           pending,
		ProvisionalCode:   t.ProvisionalCode,
		ProvisionalAmount: t.ProvisionalAmount.StringFixed(2),
		Entry:             toCriteria(t.Entry),
		Tiers:             tiers,
		Vaccination: VaccinationRecordResponse{
			LatestYear: v.LatestYear, LatestCycle: v.LatestCycle,
			VaccinatedHeads: v.VaccinatedHeads, NotVaccinated: v.NotVaccinated,
			ReportedCycles: v.ReportedCycles, CompleteCycles: v.CompleteCycles,
			VaccinationRate: v.VaccinationRate().StringFixed(4),
			CompleteRatio:   v.CompleteCycleRatio().StringFixed(4),
			HoldingActive:   v.HoldingActive,
			HistoryKnown:    v.HistoryKnown,
		},
	}
}
