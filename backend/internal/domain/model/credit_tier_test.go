package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

func yes() *bool { v := true; return &v }
func no() *bool  { v := false; return &v }

// record builds a history: heads vaccinated and left out in the latest cycle,
// cycles reported, and how many of those were complete.
func record(heads, notVaccinated, cycles, complete int) model.VaccinationRecord {
	return model.VaccinationRecord{LatestYear: 2026, LatestCycle: 1,
		VaccinatedHeads: heads, NotVaccinated: notVaccinated,
		ReportedCycles: cycles, CompleteCycles: complete, HoldingActive: true, HistoryKnown: true}
}

func evidence(rec model.VaccinationRecord) model.TierEvidence {
	return model.TierEvidence{IsCedula: true, CampaignBuyer: yes(), CleanBureau: yes(), OwnedHolding: yes(), Record: rec}
}

func TestCreditTierGrantsTheHighestLayerCleared(t *testing.T) {
	tiers := model.DefaultCreditTiers()
	cases := []struct {
		name string
		rec  model.VaccinationRecord
		want string
	}{
		{"below every layer", record(9, 0, 4, 4), ""},
		{"500k at its exact thresholds", record(10, 4, 4, 0), "500k"}, // 10/14 = 71%
		{"500k has no complete-cycle minimum", record(10, 3, 5, 0), "500k"},
		{"rate just under 70% misses 500k", record(7, 4, 4, 4), ""}, // 63.6%
		{"1M at its exact thresholds", record(30, 5, 6, 4), "1M"},   // 85.7%, 66.7%
		{"1M heads but 5 cycles stays at 500k", record(30, 0, 5, 5), "500k"},
		{"1M blocked by complete cycles", record(30, 0, 6, 3), "500k"}, // 50%
		{"2M at its exact thresholds", record(60, 6, 10, 9), "2M"},     // 90.9%, 90%
		{"2M heads, 1M history", record(100, 0, 8, 8), "1M"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := model.AssessCreditTier(evidence(c.rec), tiers)
			assert.Equal(t, c.want, got.Code)
		})
	}
}

func TestCreditTierAmounts(t *testing.T) {
	got := model.AssessCreditTier(evidence(record(60, 0, 10, 10)), model.DefaultCreditTiers())
	assert.Equal(t, "2000000", got.Amount.String())

	none := model.AssessCreditTier(evidence(record(1, 0, 1, 1)), model.DefaultCreditTiers())
	assert.True(t, none.Amount.IsZero())
}

func TestAFailedEntryFilterGrantsNothing(t *testing.T) {
	strong := record(100, 0, 12, 12)
	for name, ev := range map[string]model.TierEvidence{
		"not a cedula":     {IsCedula: false, CampaignBuyer: yes(), CleanBureau: yes(), OwnedHolding: yes(), Record: strong},
		"not a buyer":      {IsCedula: true, CampaignBuyer: no(), CleanBureau: yes(), OwnedHolding: yes(), Record: strong},
		"negative report":  {IsCedula: true, CampaignBuyer: yes(), CleanBureau: no(), OwnedHolding: yes(), Record: strong},
		"inactive holding": {IsCedula: true, CampaignBuyer: yes(), CleanBureau: yes(), OwnedHolding: yes(), Record: func() model.VaccinationRecord { r := strong; r.HoldingActive = false; return r }()},
	} {
		t.Run(name, func(t *testing.T) {
			got := model.AssessCreditTier(ev, model.DefaultCreditTiers())
			assert.False(t, got.Eligible)
			assert.Empty(t, got.Code)
			assert.True(t, got.Amount.IsZero())
		})
	}
}

func TestAnUnverifiedEntryFilterIsPendingNotFatal(t *testing.T) {
	ev := evidence(record(30, 0, 6, 6))
	ev.CleanBureau = nil // the bureaus are not queried

	got := model.AssessCreditTier(ev, model.DefaultCreditTiers())

	assert.True(t, got.Eligible)
	assert.Equal(t, "1M", got.Code)
	assert.Equal(t, []string{model.CriterionCleanBureau}, got.Pending)
}

func TestAnUnknownOwnershipHoldsBack2MAndSaysSo(t *testing.T) {
	ev := evidence(record(80, 0, 12, 12))
	ev.OwnedHolding = nil // the registry does not say who owns the land

	got := model.AssessCreditTier(ev, model.DefaultCreditTiers())

	assert.Equal(t, "1M", got.Code, "2M is never granted on an assumption")
	assert.Equal(t, "2M", got.ProvisionalCode)
	assert.Equal(t, "2000000", got.ProvisionalAmount.String())
}

func TestTheProvisionalLayerStopsAtAMissedThreshold(t *testing.T) {
	ev := evidence(record(40, 0, 12, 12)) // below 2M's 60 heads
	ev.OwnedHolding = nil

	got := model.AssessCreditTier(ev, model.DefaultCreditTiers())

	assert.Equal(t, "1M", got.Code)
	assert.Equal(t, "1M", got.ProvisionalCode, "nothing pending would lift it further")
}

func TestWithoutHistoryNoLayerIsGrantedButOneIsProvisional(t *testing.T) {
	rec := record(45, 3, 1, 1) // 93.75% vaccinated; one cycle is all the source returned
	rec.HistoryKnown = false
	ev := evidence(rec)
	ev.OwnedHolding = nil

	got := model.AssessCreditTier(ev, model.DefaultCreditTiers())

	assert.Empty(t, got.Code, "cycle counts nobody has seen are not met")
	assert.True(t, got.Amount.IsZero())
	assert.Equal(t, "1M", got.ProvisionalCode, "heads and rate reach 1M; 2M needs 60 heads")
	for _, tier := range got.Tiers {
		for _, c := range tier.Criteria {
			if c.Code == model.CriterionReportedCycles || c.Code == model.CriterionCompleteCycles {
				assert.Equal(t, model.CheckUnverified, c.Status, tier.Code+" "+c.Code)
			}
		}
	}
}

func TestWithoutHistoryAFailedThresholdStillCounts(t *testing.T) {
	rec := record(8, 0, 1, 1) // under 500k's 10 heads
	rec.HistoryKnown = false

	got := model.AssessCreditTier(evidence(rec), model.DefaultCreditTiers())

	assert.Empty(t, got.Code)
	assert.Empty(t, got.ProvisionalCode)
	assert.True(t, got.ProvisionalAmount.IsZero())
}

func TestARecordWithNoAnimalsHasAZeroRate(t *testing.T) {
	assert.True(t, model.VaccinationRecord{}.VaccinationRate().IsZero())
	assert.True(t, model.VaccinationRecord{}.CompleteCycleRatio().IsZero())
}
