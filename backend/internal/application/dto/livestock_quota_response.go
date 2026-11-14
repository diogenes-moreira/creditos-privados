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

// QuotaTierReasonResponse is one unmet requirement behind the tier result.
type QuotaTierReasonResponse struct {
	// Scope is "entry" for the entry filter, or the code of the tier whose
	// requirement was not met (the next tier up from the one granted).
	Scope string `json:"scope"`
	// Requirement is cedula, currentCycleRegistered, activeHolding,
	// vaccinatedHeads, vaccinationRate, ownedHolding or vaccinatedCycles.
	Requirement string `json:"requirement"`
	Required    string `json:"required"`
	Actual      string `json:"actual"`
}

// QuotaTierResponse is the pilot tier decision ("Escalones Piloto").
type QuotaTierResponse struct {
	// Code is tier1, tier2, tier3 or notEligible.
	Code          string `json:"code"`
	Label         string `json:"label"`
	Amount        string `json:"amount"`
	PolicyVersion string `json:"policyVersion"`
	// Reasons are the failed entry filters, or what kept the rancher from the
	// next tier up; empty at the top tier.
	Reasons []QuotaTierReasonResponse `json:"reasons"`
	// CurrentCycle is the RUV cycle measured ("2026-1"); CurrentCycleFound
	// whether any predio was recorded in it.
	CurrentCycle      string `json:"currentCycle"`
	CurrentCycleFound bool   `json:"currentCycleFound"`
	VaccinatedHeads   int    `json:"vaccinatedHeads"`
	NotVaccinated     int    `json:"notVaccinated"`
	// VaccinationRate is a fraction with four decimals, e.g. "0.8500".
	VaccinationRate string `json:"vaccinationRate"`
	OwnedHolding    bool   `json:"ownedHolding"`
}

// LivestockQuotaResponse is the assigned quota with the trace of how it was reached.
type LivestockQuotaResponse struct {
	ID         string `json:"id"`
	NationalID string `json:"nationalId"`
	Reference  string `json:"reference"`
	// Quota is the amount granted: the tier amount under the tiers strategy
	// (zero when not eligible), the engine's quota under the engine strategy.
	Quota    string `json:"quota"`
	Currency string `json:"currency"`
	// Strategy is how Quota was decided: tiers or engine.
	Strategy string `json:"strategy"`
	// EngineQuota is what the individual engine computes, under either
	// strategy, so the two can be compared.
	EngineQuota string `json:"engineQuota"`
	// Tier is the tier decision; absent under the engine strategy.
	Tier *QuotaTierResponse `json:"tier,omitempty"`
	// ActiveCeiling is what bound the quota: the engine's ceiling name, or
	// the tier label ("Escalón 1", "No elegible") under the tiers strategy.
	ActiveCeiling string `json:"activeCeiling"`
	// ActiveCeilingCode is the stable key of ActiveCeiling: ltv, capacity,
	// inputs or policyCap, or the tier code under the tiers strategy. Branch
	// on this, not on the display text.
	ActiveCeilingCode string                         `json:"activeCeilingCode"`
	Ceilings          LivestockQuotaCeilingsResponse `json:"ceilings"`
	TermMonths        int                            `json:"termMonths"`
	PeriodRatePct     string                         `json:"periodRatePct"`
	Score             LivestockScoreResponse         `json:"score"`
	Typology          string                         `json:"typology"`
	Head              int                            `json:"head"`
	UGG               string                         `json:"ugg"`
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

// ToLivestockQuotaResponse maps a stored assessment, its engine result and,
// under the tiers strategy, the tier decision (nil otherwise).
func ToLivestockQuotaResponse(r *model.LivestockQuotaAssessment, a model.LivestockAssessment, tier *model.TierAssessment) LivestockQuotaResponse {
	q := a.Quota
	activeCeiling, activeCeilingCode := q.ActiveCeiling, q.ActiveCeilingCode
	strategy := string(model.QuotaStrategyEngine)
	if tier != nil {
		activeCeiling, activeCeilingCode = tier.Label, tier.Code
		strategy = string(model.QuotaStrategyTiers)
	}
	return LivestockQuotaResponse{
		ID:                r.ID.String(),
		NationalID:        r.NationalID,
		Reference:         r.Reference,
		Quota:             r.Quota.StringFixed(2),
		Currency:          r.Currency,
		Strategy:          strategy,
		EngineQuota:       r.EngineQuota.StringFixed(2),
		Tier:              toQuotaTierResponse(tier),
		ActiveCeiling:     activeCeiling,
		ActiveCeilingCode: activeCeilingCode,
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

func toQuotaTierResponse(t *model.TierAssessment) *QuotaTierResponse {
	if t == nil {
		return nil
	}
	reasons := make([]QuotaTierReasonResponse, 0, len(t.Reasons))
	for _, r := range t.Reasons {
		reasons = append(reasons, QuotaTierReasonResponse{
			Scope: r.Scope, Requirement: r.Requirement, Required: r.Required, Actual: r.Actual,
		})
	}
	return &QuotaTierResponse{
		Code:              t.Code,
		Label:             t.Label,
		Amount:            t.Amount.StringFixed(2),
		PolicyVersion:     t.PolicyVersion,
		Reasons:           reasons,
		CurrentCycle:      t.CurrentCycle,
		CurrentCycleFound: t.CurrentCycleFound,
		VaccinatedHeads:   t.VaccinatedHeads,
		NotVaccinated:     t.NotVaccinated,
		VaccinationRate:   t.VaccinationRate.StringFixed(4),
		OwnedHolding:      t.OwnedHolding,
	}
}
