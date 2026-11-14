package port_test

import (
	"time"

	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/stretchr/testify/assert"
)

func TestLatestHoldingsKeepsEachPredioAtItsMostRecentCycle(t *testing.T) {
	r := port.RUVRancher{Holdings: []port.RUVHolding{
		{Code: "A", Department: "CAUCA", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 10}},
		{Code: "A", Department: "CAUCA", Year: 2025, Cycle: 2, Herd: model.Herd{MalesOver3Years: 99}},
		{Code: "B", Department: "CESAR", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 4}},
		{Code: "A", Department: "CAUCA", Year: 2026, Cycle: 2, Herd: model.Herd{MalesOver3Years: 12}},
	}}

	latest := r.LatestHoldings()
	assert.Len(t, latest, 2)
	assert.Equal(t, 12, latest[0].Herd.MalesOver3Years)
	assert.Equal(t, 16, r.AggregatedHerd().MalesOver3Years)
	assert.Equal(t, "CAUCA", r.Department())
	assert.Equal(t, 3, r.ReportedCycles(), "2025-2, 2026-1 and 2026-2")
}

func TestVaccinationRecordReadsTheLatestCycleAndTheHistory(t *testing.T) {
	r := port.RUVRancher{Holdings: []port.RUVHolding{
		{Code: "A", Year: 2025, Cycle: 1, Herd: model.Herd{MalesOver3Years: 20, NotVaccinated: 2}},
		{Code: "A", Year: 2025, Cycle: 2, Herd: model.Herd{MalesOver3Years: 22}},
		{Code: "A", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 25, NotVaccinated: 1}},
		// A second holding vaccinated only in the latest cycle.
		{Code: "B", Year: 2026, Cycle: 1, Herd: model.Herd{Females3To5Years: 10}},
	}, HistoryComplete: true}

	rec := r.VaccinationRecord(time.Date(2026, time.October, 27, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 2026, rec.LatestYear)
	assert.Equal(t, 1, rec.LatestCycle)
	assert.Equal(t, 35, rec.VaccinatedHeads, "both holdings in the latest cycle")
	assert.Equal(t, 1, rec.NotVaccinated)
	assert.Equal(t, 3, rec.ReportedCycles)
	assert.Equal(t, 1, rec.CompleteCycles, "only 2025-2 left no animal out")
	assert.True(t, rec.HoldingActive)
	assert.True(t, rec.HistoryKnown)
}

func TestVaccinationRecordKnowsWhenTheHistoryIsMissing(t *testing.T) {
	r := port.RUVRancher{Holdings: []port.RUVHolding{
		{Code: "A", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 20}},
	}}

	assert.False(t, r.VaccinationRecord(time.Date(2026, time.October, 27, 0, 0, 0, 0, time.UTC)).HistoryKnown,
		"one cycle from a source that only returns the current one says nothing about the past")
}

func TestVaccinationRecordCallsAHoldingInactiveAfterTwoMissedCycles(t *testing.T) {
	r := port.RUVRancher{Holdings: []port.RUVHolding{
		{Code: "A", Year: 2025, Cycle: 1, Herd: model.Herd{MalesOver3Years: 20}},
	}}

	// In October 2026 the current cycle is 2026-1; 2025-2 would still count.
	assert.False(t, r.VaccinationRecord(time.Date(2026, time.October, 27, 0, 0, 0, 0, time.UTC)).HoldingActive)
	// Before May the latest due cycle is last year's second.
	assert.True(t, r.VaccinationRecord(time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)).HoldingActive)
}
