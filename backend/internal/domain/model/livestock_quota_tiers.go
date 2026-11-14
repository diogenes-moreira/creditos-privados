package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// The quota tiers ("Escalones Piloto") are the pilot's pre-approval rule, an
// alternative to the individual engine: instead of pricing the herd, a rancher
// qualifies for one of three fixed amounts by clearing every requirement of
// that tier, after passing an entry filter common to all of them. The highest
// tier cleared wins; clearing none means not eligible, a quota of zero.
//
// The pilot also requires a clean report in the credit bureaus and having
// bought from TVGAN in 2026. Neither is known to Prestia: in v1 the partner
// (CuppoGan / TVGAN) checks them outside this service, so they are not
// evaluated here.

// QuotaStrategy selects how the livestock quota is decided.
type QuotaStrategy string

const (
	// QuotaStrategyTiers grants the pilot's fixed tier amounts. Default.
	QuotaStrategyTiers QuotaStrategy = "tiers"
	// QuotaStrategyEngine grants what the individual engine computes.
	QuotaStrategyEngine QuotaStrategy = "engine"
)

// ParseQuotaStrategy reads a strategy name; empty means the default, tiers.
// Anything else is an error rather than a silent fallback, because running the
// wrong strategy changes every quota granted.
func ParseQuotaStrategy(s string) (QuotaStrategy, error) {
	switch QuotaStrategy(strings.ToLower(strings.TrimSpace(s))) {
	case "", QuotaStrategyTiers:
		return QuotaStrategyTiers, nil
	case QuotaStrategyEngine:
		return QuotaStrategyEngine, nil
	default:
		return "", fmt.Errorf("unknown livestock quota strategy %q: use %q or %q", s, QuotaStrategyTiers, QuotaStrategyEngine)
	}
}

// VaccinationCycle is one RUV vaccination cycle. The registry runs two a year.
type VaccinationCycle struct {
	Year  int
	Cycle int
}

// DefaultCurrentCycle is the cycle the v1 tiers are measured on unless the
// deployment says otherwise.
var DefaultCurrentCycle = VaccinationCycle{Year: 2026, Cycle: 1}

func (c VaccinationCycle) String() string { return fmt.Sprintf("%d-%d", c.Year, c.Cycle) }

// ParseVaccinationCycle reads a cycle as "YYYY-C" (e.g. "2026-1"); empty means
// DefaultCurrentCycle.
func ParseVaccinationCycle(s string) (VaccinationCycle, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultCurrentCycle, nil
	}
	year, cycle, ok := strings.Cut(s, "-")
	y, errY := strconv.Atoi(year)
	c, errC := strconv.Atoi(cycle)
	if !ok || errY != nil || errC != nil || y < 2000 || y > 2100 || (c != 1 && c != 2) {
		return VaccinationCycle{}, fmt.Errorf("invalid vaccination cycle %q: use YYYY-C, with C 1 or 2", s)
	}
	return VaccinationCycle{Year: y, Cycle: c}, nil
}

// Tier codes, stable for the API and the stored record.
const (
	TierOne         = "tier1"
	TierTwo         = "tier2"
	TierThree       = "tier3"
	TierNotEligible = "notEligible"

	TierNotEligibleLabel = "No elegible"
)

// Tier policy versions. v2 adds the vaccinated-cycles history requirement.
const (
	TierPolicyV1 = "v1"
	TierPolicyV2 = "v2"
)

// RUVHoldingOwned is what the tiers take as the land tenure of a rancher read
// from the RUV when no record's "propietario" names him as owner, including
// when it is empty, as FEDEGÁN sends it today. Not owned is the conservative
// reading (the engine likewise assumes leased), and it leaves tier 3 out of
// reach until the registry fills the field.
const RUVHoldingOwned = false

// QuotaTier is one row of the tier table.
type QuotaTier struct {
	Code   string
	Label  string
	Amount decimal.Decimal
	// MinVaccinatedHeads is counted over the current cycle, every predio of
	// the national ID summed.
	MinVaccinatedHeads int
	// MinVaccinationRate is vaccinated / (vaccinated + not vaccinated) in the
	// current cycle, as a fraction rounded to four decimals.
	MinVaccinationRate decimal.Decimal
	// RequiresOwnedHolding is true when the land must belong to the rancher.
	RequiresOwnedHolding bool
	// MinVaccinatedCycles is how many of the last ten cycles must show
	// vaccination. It belongs to v2 and is ignored by a v1 policy, because the
	// registry query only returns the current cycle.
	MinVaccinatedCycles int
}

// QuotaTierPolicy is the tier table with its version. The thresholds are code
// defaults, not operator-edited: the pilot fixes them by version.
type QuotaTierPolicy struct {
	Version string
	// Tiers is ordered lowest first.
	Tiers []QuotaTier
}

// DefaultQuotaTierPolicy is "Escalones Piloto" v1, November 2026.
func DefaultQuotaTierPolicy() QuotaTierPolicy {
	return QuotaTierPolicy{
		Version: TierPolicyV1,
		Tiers: []QuotaTier{
			{Code: TierOne, Label: "Escalón 1", Amount: decimal.NewFromInt(500_000),
				MinVaccinatedHeads: 10, MinVaccinationRate: decimal.RequireFromString("0.70"),
				MinVaccinatedCycles: 1},
			{Code: TierTwo, Label: "Escalón 2", Amount: decimal.NewFromInt(1_000_000),
				MinVaccinatedHeads: 30, MinVaccinationRate: decimal.RequireFromString("0.85"),
				MinVaccinatedCycles: 4},
			{Code: TierThree, Label: "Escalón 3", Amount: decimal.NewFromInt(2_000_000),
				MinVaccinatedHeads: 60, MinVaccinationRate: decimal.RequireFromString("0.90"),
				RequiresOwnedHolding: true, MinVaccinatedCycles: 6},
		},
	}
}

// TierEvidence is what the tiers are decided on, already measured over the
// current cycle.
type TierEvidence struct {
	// IDTypeKnown is false when the caller sent no document type (a herd
	// supplied by hand); the cédula filter is then skipped.
	IDTypeKnown bool
	IsCedula    bool
	// CurrentCycle is the cycle measured; CurrentCycleRegistered is whether at
	// least one predio was vaccinated in it.
	CurrentCycle           VaccinationCycle
	CurrentCycleRegistered bool
	// VaccinatedHeads and NotVaccinated are summed over the predios recorded
	// in the current cycle, each predio once.
	VaccinatedHeads int
	NotVaccinated   int
	// HoldingActive: the RUV reports no holding status, so registry lookups
	// assume active, as the engine does.
	HoldingActive bool
	OwnedHolding  bool
	// VaccinatedCycles among the last ten; only read by a v2 policy.
	VaccinatedCycles int
}

// VaccinationRate is vaccinated over all animals in the current cycle, rounded
// to four decimals; zero when the cycle reports no animals.
func (e TierEvidence) VaccinationRate() decimal.Decimal {
	total := e.VaccinatedHeads + e.NotVaccinated
	if total == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(e.VaccinatedHeads)).
		DivRound(decimal.NewFromInt(int64(total)), 8).Round(4)
}

// Requirement codes, stable for the API.
const (
	RequirementCedula                 = "cedula"
	RequirementCurrentCycleRegistered = "currentCycleRegistered"
	RequirementActiveHolding          = "activeHolding"
	RequirementVaccinatedHeads        = "vaccinatedHeads"
	RequirementVaccinationRate        = "vaccinationRate"
	RequirementOwnedHolding           = "ownedHolding"
	RequirementVaccinatedCycles       = "vaccinatedCycles"
)

// TierScopeEntry marks a reason that comes from the entry filter rather than
// from a tier's own requirements.
const TierScopeEntry = "entry"

// TierReason is one unmet requirement that explains the result: an entry
// filter that failed, or what kept the rancher from the next tier up.
type TierReason struct {
	// Scope is TierScopeEntry or the code of the tier the requirement belongs to.
	Scope       string
	Requirement string
	Required    string
	Actual      string
}

// TierAssessment is the tier granted and why.
type TierAssessment struct {
	Code          string
	Label         string
	Amount        decimal.Decimal
	PolicyVersion string
	// Reasons lists the failed entry filters when the rancher did not pass
	// them, and otherwise the unmet requirements of the next tier up (empty
	// at the top tier).
	Reasons []TierReason

	CurrentCycle      string
	CurrentCycleFound bool
	VaccinatedHeads   int
	NotVaccinated     int
	VaccinationRate   decimal.Decimal
	OwnedHolding      bool
}

func ratePct(d decimal.Decimal) string { return d.StringFixed(4) }

// unmet lists the tier's requirements the evidence does not meet.
func (p QuotaTierPolicy) unmet(t QuotaTier, e TierEvidence, rate decimal.Decimal) []TierReason {
	var out []TierReason
	if e.VaccinatedHeads < t.MinVaccinatedHeads {
		out = append(out, TierReason{Scope: t.Code, Requirement: RequirementVaccinatedHeads,
			Required: fmt.Sprintf(">=%d", t.MinVaccinatedHeads), Actual: strconv.Itoa(e.VaccinatedHeads)})
	}
	if rate.LessThan(t.MinVaccinationRate) {
		out = append(out, TierReason{Scope: t.Code, Requirement: RequirementVaccinationRate,
			Required: ">=" + ratePct(t.MinVaccinationRate), Actual: ratePct(rate)})
	}
	if t.RequiresOwnedHolding && !e.OwnedHolding {
		out = append(out, TierReason{Scope: t.Code, Requirement: RequirementOwnedHolding,
			Required: "true", Actual: "false"})
	}
	if p.Version != TierPolicyV1 && e.VaccinatedCycles < t.MinVaccinatedCycles {
		out = append(out, TierReason{Scope: t.Code, Requirement: RequirementVaccinatedCycles,
			Required: fmt.Sprintf(">=%d", t.MinVaccinatedCycles), Actual: strconv.Itoa(e.VaccinatedCycles)})
	}
	return out
}

// Assess decides the tier for the evidence.
func (p QuotaTierPolicy) Assess(e TierEvidence) TierAssessment {
	rate := e.VaccinationRate()
	result := TierAssessment{
		Code:              TierNotEligible,
		Label:             TierNotEligibleLabel,
		Amount:            decimal.Zero,
		PolicyVersion:     p.Version,
		Reasons:           []TierReason{},
		CurrentCycle:      e.CurrentCycle.String(),
		CurrentCycleFound: e.CurrentCycleRegistered,
		VaccinatedHeads:   e.VaccinatedHeads,
		NotVaccinated:     e.NotVaccinated,
		VaccinationRate:   rate,
		OwnedHolding:      e.OwnedHolding,
	}

	if e.IDTypeKnown && !e.IsCedula {
		result.Reasons = append(result.Reasons, TierReason{Scope: TierScopeEntry,
			Requirement: RequirementCedula, Required: "true", Actual: "false"})
	}
	if !e.CurrentCycleRegistered {
		result.Reasons = append(result.Reasons, TierReason{Scope: TierScopeEntry,
			Requirement: RequirementCurrentCycleRegistered, Required: e.CurrentCycle.String(), Actual: "none"})
	}
	if !e.HoldingActive {
		result.Reasons = append(result.Reasons, TierReason{Scope: TierScopeEntry,
			Requirement: RequirementActiveHolding, Required: "true", Actual: "false"})
	}
	if len(result.Reasons) > 0 {
		return result
	}

	granted := -1
	for i, t := range p.Tiers {
		if len(p.unmet(t, e, rate)) == 0 {
			granted = i
		}
	}
	if granted >= 0 {
		t := p.Tiers[granted]
		result.Code, result.Label, result.Amount = t.Code, t.Label, t.Amount
	}
	if next := granted + 1; next < len(p.Tiers) {
		result.Reasons = p.unmet(p.Tiers[next], e, rate)
	}
	return result
}
