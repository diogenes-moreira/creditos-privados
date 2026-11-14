package model

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// The credit tiers are the pilot's pre-approval rule: a rancher qualifies for
// a fixed amount (500k, 1M or 2M) by clearing every threshold of that layer,
// all of them read from the vaccination registry, after passing an entry
// filter that applies to every layer.
//
// Some inputs cannot be read from the registry: whether the rancher has a
// negative report in the credit bureaus, whether the holding is owned, and
// (from the registry's current endpoint, which returns only the latest cycle)
// the cycle history.
// Those are never assumed. An entry filter that cannot be verified is
// returned as a pending condition of the pre-approval; a layer criterion that
// cannot be verified keeps the layer from being granted, and the layer is
// reported as provisional: reachable once it is confirmed.

// CheckStatus is the outcome of one criterion.
type CheckStatus string

const (
	CheckMet        CheckStatus = "met"
	CheckUnmet      CheckStatus = "unmet"
	CheckUnverified CheckStatus = "unverified"
)

// checkOf turns a known/unknown boolean into a status.
func checkOf(v *bool) CheckStatus {
	switch {
	case v == nil:
		return CheckUnverified
	case *v:
		return CheckMet
	default:
		return CheckUnmet
	}
}

func checkIf(ok bool) CheckStatus {
	if ok {
		return CheckMet
	}
	return CheckUnmet
}

// Criterion codes, stable for the API and translations.
const (
	CriterionCedula          = "cedula"
	CriterionCampaignBuyer   = "campaignBuyer"
	CriterionCleanBureau     = "cleanBureau"
	CriterionActiveHolding   = "activeHolding"
	CriterionVaccinatedHeads = "vaccinatedHeads"
	CriterionReportedCycles  = "reportedCycles"
	CriterionVaccinationRate = "vaccinationRate"
	CriterionCompleteCycles  = "completeCycles"
	CriterionOwnedHolding    = "ownedHolding"
)

// CreditTier is one layer of the pre-approval table.
type CreditTier struct {
	Code   string
	Amount decimal.Decimal
	// MinVaccinatedHeads is counted over the latest vaccination cycle.
	MinVaccinatedHeads int
	MinReportedCycles  int
	// MinVaccinationRate is vaccinated / (vaccinated + not vaccinated) in the
	// latest cycle, as a fraction.
	MinVaccinationRate decimal.Decimal
	// MinCompleteCycleRatio is complete cycles over reported ones, as a
	// fraction. Zero means no minimum.
	MinCompleteCycleRatio decimal.Decimal
	// RequiresOwnedHolding is true when an active holding is not enough and
	// it has to belong to the rancher.
	RequiresOwnedHolding bool
}

// DefaultCreditTiers is the pilot's table, lowest layer first.
func DefaultCreditTiers() []CreditTier {
	pct := func(v int64) decimal.Decimal { return decimal.NewFromInt(v).Div(decimal.NewFromInt(100)) }
	return []CreditTier{
		{Code: "500k", Amount: decimal.NewFromInt(500_000),
			MinVaccinatedHeads: 10, MinReportedCycles: 4,
			MinVaccinationRate: pct(70), MinCompleteCycleRatio: decimal.Zero},
		{Code: "1M", Amount: decimal.NewFromInt(1_000_000),
			MinVaccinatedHeads: 30, MinReportedCycles: 6,
			MinVaccinationRate: pct(85), MinCompleteCycleRatio: pct(65)},
		{Code: "2M", Amount: decimal.NewFromInt(2_000_000),
			MinVaccinatedHeads: 60, MinReportedCycles: 10,
			MinVaccinationRate: pct(90), MinCompleteCycleRatio: pct(90),
			RequiresOwnedHolding: true},
	}
}

// VaccinationRecord is what the registry shows about a rancher's history,
// reduced to the figures the tiers are written against.
type VaccinationRecord struct {
	// LatestYear and LatestCycle identify the most recent cycle on record.
	LatestYear  int
	LatestCycle int
	// VaccinatedHeads and NotVaccinated are summed over every holding
	// reported in the latest cycle.
	VaccinatedHeads int
	NotVaccinated   int
	ReportedCycles  int
	// CompleteCycles counts reported cycles in which no animal was left
	// unvaccinated.
	CompleteCycles int
	// HoldingActive is whether the rancher vaccinated in one of the last two
	// cycles, the registry's only sign that the holding is still operating.
	HoldingActive bool
	// HistoryKnown is whether the source returned every past cycle. Without
	// it ReportedCycles and CompleteCycles only count what was returned, and
	// the criteria built on them cannot be judged.
	HistoryKnown bool
}

// VaccinationRate is the share of the latest cycle's animals that were
// vaccinated; zero when the cycle reports no animals.
func (r VaccinationRecord) VaccinationRate() decimal.Decimal {
	total := r.VaccinatedHeads + r.NotVaccinated
	if total == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(r.VaccinatedHeads)).Div(decimal.NewFromInt(int64(total)))
}

// CompleteCycleRatio is complete cycles over reported ones; zero with none.
func (r VaccinationRecord) CompleteCycleRatio() decimal.Decimal {
	if r.ReportedCycles == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(r.CompleteCycles)).Div(decimal.NewFromInt(int64(r.ReportedCycles)))
}

// TierEvidence is everything the tiers are decided on. Pointers are facts the
// caller may not know: nil means unverified, never false.
type TierEvidence struct {
	// IsCedula is whether the rancher was identified by cédula de ciudadanía.
	IsCedula      bool
	CampaignBuyer *bool
	CleanBureau   *bool
	OwnedHolding  *bool
	Record        VaccinationRecord
}

// TierCriterion is one criterion's outcome, with the figures behind it so the
// screen can say why a layer was or was not reached.
type TierCriterion struct {
	Code     string
	Status   CheckStatus
	Actual   string
	Required string
}

// TierEvaluation is one layer checked against the evidence.
type TierEvaluation struct {
	Code      string
	Amount    decimal.Decimal
	Qualifies bool
	Criteria  []TierCriterion
}

// TierAssessment is the pre-approval: the highest layer cleared, and what is
// still pending.
type TierAssessment struct {
	// Code and Amount are the layer granted; empty and zero when none is.
	Code   string
	Amount decimal.Decimal
	// Eligible is false when an entry filter is known to fail, in which case
	// no layer is granted whatever the registry shows.
	Eligible bool
	Entry    []TierCriterion
	Tiers    []TierEvaluation
	// Pending lists the entry filters that could not be verified: the
	// pre-approval holds only once someone confirms them.
	Pending []string
	// ProvisionalCode and ProvisionalAmount are the highest layer no known
	// fact rules out: every criterion is met or still unverified. It is what
	// the rancher reaches if everything pending is confirmed, and equals the
	// granted layer when nothing is pending.
	ProvisionalCode   string
	ProvisionalAmount decimal.Decimal
}

func pctString(d decimal.Decimal) string {
	return d.Mul(decimal.NewFromInt(100)).Round(1).String() + "%"
}

// AssessCreditTier decides the layer for the evidence.
func AssessCreditTier(ev TierEvidence, tiers []CreditTier) TierAssessment {
	rec := ev.Record
	entry := []TierCriterion{
		{Code: CriterionCedula, Status: checkIf(ev.IsCedula)},
		{Code: CriterionCampaignBuyer, Status: checkOf(ev.CampaignBuyer)},
		{Code: CriterionCleanBureau, Status: checkOf(ev.CleanBureau)},
		{Code: CriterionActiveHolding, Status: checkIf(rec.HoldingActive),
			Actual: fmt.Sprintf("%d-%d", rec.LatestYear, rec.LatestCycle)},
	}

	result := TierAssessment{Eligible: true, Entry: entry, Amount: decimal.Zero, ProvisionalAmount: decimal.Zero}
	for _, c := range entry {
		switch c.Status {
		case CheckUnmet:
			result.Eligible = false
		case CheckUnverified:
			result.Pending = append(result.Pending, c.Code)
		}
	}

	rate, complete := rec.VaccinationRate(), rec.CompleteCycleRatio()
	// A criterion read from the cycle history is unverified, not failed,
	// when the source did not return the history.
	history := func(ok bool) CheckStatus {
		if !rec.HistoryKnown {
			return CheckUnverified
		}
		return checkIf(ok)
	}
	for _, t := range tiers {
		criteria := []TierCriterion{
			{Code: CriterionVaccinatedHeads, Status: checkIf(rec.VaccinatedHeads >= t.MinVaccinatedHeads),
				Actual: fmt.Sprint(rec.VaccinatedHeads), Required: fmt.Sprintf("≥%d", t.MinVaccinatedHeads)},
			{Code: CriterionReportedCycles, Status: history(rec.ReportedCycles >= t.MinReportedCycles),
				Actual: fmt.Sprint(rec.ReportedCycles), Required: fmt.Sprintf("≥%d", t.MinReportedCycles)},
			{Code: CriterionVaccinationRate, Status: checkIf(rate.GreaterThanOrEqual(t.MinVaccinationRate)),
				Actual: pctString(rate), Required: "≥" + pctString(t.MinVaccinationRate)},
		}
		if t.MinCompleteCycleRatio.IsPositive() {
			criteria = append(criteria, TierCriterion{Code: CriterionCompleteCycles,
				Status: history(complete.GreaterThanOrEqual(t.MinCompleteCycleRatio)),
				Actual: pctString(complete), Required: "≥" + pctString(t.MinCompleteCycleRatio)})
		}
		if t.RequiresOwnedHolding {
			criteria = append(criteria, TierCriterion{Code: CriterionOwnedHolding, Status: checkOf(ev.OwnedHolding)})
		}

		qualifies, blockedOnlyByUnverified := true, true
		for _, c := range criteria {
			if c.Status != CheckMet {
				qualifies = false
			}
			if c.Status == CheckUnmet {
				blockedOnlyByUnverified = false
			}
		}
		result.Tiers = append(result.Tiers, TierEvaluation{Code: t.Code, Amount: t.Amount, Qualifies: qualifies, Criteria: criteria})

		if !result.Eligible {
			continue
		}
		if qualifies {
			result.Code, result.Amount = t.Code, t.Amount
		}
		if blockedOnlyByUnverified {
			result.ProvisionalCode, result.ProvisionalAmount = t.Code, t.Amount
		}
	}
	return result
}
