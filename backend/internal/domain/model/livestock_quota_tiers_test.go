package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eligible is a rancher who passes the entry filter, with the given herd in
// the current cycle.
func eligible(vaccinated, notVaccinated int, owned bool) model.TierEvidence {
	return model.TierEvidence{
		IDTypeKnown: true, IsCedula: true,
		CurrentCycle: model.DefaultCurrentCycle, CurrentCycleRegistered: true,
		VaccinatedHeads: vaccinated, NotVaccinated: notVaccinated,
		HoldingActive: true, OwnedHolding: owned,
	}
}

func assessTier(e model.TierEvidence) model.TierAssessment {
	return model.DefaultQuotaTierPolicy().Assess(e)
}

func TestTierPolicyV1IsThePilotTable(t *testing.T) {
	p := model.DefaultQuotaTierPolicy()
	assert.Equal(t, model.TierPolicyV1, p.Version)
	require.Len(t, p.Tiers, 3)
	for i, want := range []struct {
		code, amount, rate string
		heads              int
		owned              bool
	}{
		{"tier1", "500000", "0.7", 10, false},
		{"tier2", "1000000", "0.85", 30, false},
		{"tier3", "2000000", "0.9", 60, true},
	} {
		tier := p.Tiers[i]
		assert.Equal(t, want.code, tier.Code)
		assert.Equal(t, want.amount, tier.Amount.String())
		assert.Equal(t, want.rate, tier.MinVaccinationRate.String())
		assert.Equal(t, want.heads, tier.MinVaccinatedHeads)
		assert.Equal(t, want.owned, tier.RequiresOwnedHolding)
	}
}

func TestTierHeadsBoundary(t *testing.T) {
	below := assessTier(eligible(9, 0, false))
	assert.Equal(t, model.TierNotEligible, below.Code)
	assert.True(t, below.Amount.IsZero())
	assert.Equal(t, []model.TierReason{{Scope: "tier1", Requirement: model.RequirementVaccinatedHeads,
		Required: ">=10", Actual: "9"}}, below.Reasons)

	at := assessTier(eligible(10, 0, false))
	assert.Equal(t, model.TierOne, at.Code)
	assert.Equal(t, "500000", at.Amount.String())
	assert.Equal(t, "Escalón 1", at.Label)
}

func TestTierVaccinationRateBoundary(t *testing.T) {
	// 6999 / 10000 = 0.6999: short of 0.70.
	below := assessTier(eligible(6999, 3001, false))
	assert.Equal(t, "0.6999", below.VaccinationRate.StringFixed(4))
	assert.Equal(t, model.TierNotEligible, below.Code)
	assert.Equal(t, []model.TierReason{{Scope: "tier1", Requirement: model.RequirementVaccinationRate,
		Required: ">=0.7000", Actual: "0.6999"}}, below.Reasons)

	// 14 / 20 = 0.70 exactly.
	at := assessTier(eligible(14, 6, false))
	assert.Equal(t, "0.7000", at.VaccinationRate.StringFixed(4))
	assert.Equal(t, model.TierOne, at.Code)

	// The rate is rounded to four decimals before comparing: 0.69995 counts as 0.70.
	rounded := assessTier(eligible(13999, 6001, false))
	assert.Equal(t, "0.7000", rounded.VaccinationRate.StringFixed(4))
	assert.Equal(t, model.TierOne, rounded.Code)
}

func TestTierThreeNeedsSixtyHeadsAndAnOwnedHolding(t *testing.T) {
	owned59 := assessTier(eligible(59, 0, true))
	assert.Equal(t, model.TierTwo, owned59.Code)
	assert.Equal(t, []model.TierReason{{Scope: "tier3", Requirement: model.RequirementVaccinatedHeads,
		Required: ">=60", Actual: "59"}}, owned59.Reasons)

	owned60 := assessTier(eligible(60, 0, true))
	assert.Equal(t, model.TierThree, owned60.Code)
	assert.Equal(t, "2000000", owned60.Amount.String())
	assert.Empty(t, owned60.Reasons, "nothing above the top tier")

	leased60 := assessTier(eligible(60, 0, false))
	assert.Equal(t, model.TierTwo, leased60.Code)
	assert.Equal(t, []model.TierReason{{Scope: "tier3", Requirement: model.RequirementOwnedHolding,
		Required: "true", Actual: "false"}}, leased60.Reasons)
}

func TestTheHighestTierWhoseRequirementsAreAllMetWins(t *testing.T) {
	// 100 heads at 86%: tier 2's 85% is met, tier 3's 90% is not.
	got := assessTier(eligible(86, 14, true))
	assert.Equal(t, model.TierTwo, got.Code)
	assert.Equal(t, "1000000", got.Amount.String())
	assert.Equal(t, []model.TierReason{{Scope: "tier3", Requirement: model.RequirementVaccinationRate,
		Required: ">=0.9000", Actual: "0.8600"}}, got.Reasons)

	// Enough heads for tier 2 but a rate only good for tier 1.
	assert.Equal(t, model.TierOne, assessTier(eligible(40, 10, false)).Code)
}

func TestTierEntryFilter(t *testing.T) {
	nit := eligible(100, 0, true)
	nit.IsCedula = false
	got := assessTier(nit)
	assert.Equal(t, model.TierNotEligible, got.Code)
	assert.True(t, got.Amount.IsZero())
	assert.Equal(t, []model.TierReason{{Scope: model.TierScopeEntry, Requirement: model.RequirementCedula,
		Required: "true", Actual: "false"}}, got.Reasons)

	// Without a document type (a herd supplied by hand) the filter is skipped.
	unknown := nit
	unknown.IDTypeKnown = false
	assert.Equal(t, model.TierThree, assessTier(unknown).Code)

	noCycle := eligible(0, 0, false)
	noCycle.CurrentCycleRegistered = false
	got = assessTier(noCycle)
	assert.Equal(t, model.TierNotEligible, got.Code)
	assert.False(t, got.CurrentCycleFound)
	assert.Equal(t, []model.TierReason{{Scope: model.TierScopeEntry, Requirement: model.RequirementCurrentCycleRegistered,
		Required: "2026-1", Actual: "none"}}, got.Reasons)

	inactive := eligible(100, 0, true)
	inactive.HoldingActive = false
	assert.Equal(t, model.TierNotEligible, assessTier(inactive).Code)
}

func TestTierCyclesRequirementIsIgnoredInV1(t *testing.T) {
	e := eligible(100, 0, true)
	e.VaccinatedCycles = 0
	assert.Equal(t, model.TierThree, assessTier(e).Code)

	v2 := model.DefaultQuotaTierPolicy()
	v2.Version = model.TierPolicyV2
	got := v2.Assess(e)
	assert.Equal(t, model.TierNotEligible, got.Code)
	assert.Equal(t, model.RequirementVaccinatedCycles, got.Reasons[0].Requirement)
}

func TestRUVTenureCountsAsNotOwned(t *testing.T) {
	assert.False(t, model.RUVHoldingOwned, "an empty propietario must not count as owned")
}

func TestParseQuotaStrategy(t *testing.T) {
	for in, want := range map[string]model.QuotaStrategy{
		"": model.QuotaStrategyTiers, "tiers": model.QuotaStrategyTiers, " Tiers ": model.QuotaStrategyTiers,
		"engine": model.QuotaStrategyEngine, "ENGINE": model.QuotaStrategyEngine,
	} {
		got, err := model.ParseQuotaStrategy(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := model.ParseQuotaStrategy("escalones")
	assert.Error(t, err)
}

func TestParseVaccinationCycle(t *testing.T) {
	got, err := model.ParseVaccinationCycle("")
	require.NoError(t, err)
	assert.Equal(t, model.VaccinationCycle{Year: 2026, Cycle: 1}, got)

	got, err = model.ParseVaccinationCycle("2026-2")
	require.NoError(t, err)
	assert.Equal(t, "2026-2", got.String())

	for _, bad := range []string{"2026", "2026-3", "2026-0", "26-1", "2026/1", "abcd-1"} {
		_, err := model.ParseVaccinationCycle(bad)
		assert.Error(t, err, bad)
	}
}
