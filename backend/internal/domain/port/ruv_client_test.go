package port_test

import (
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

func TestCycleHerdSumsThePrediosOfThatCycleOnce(t *testing.T) {
	r := port.RUVRancher{Holdings: []port.RUVHolding{
		{Code: "A", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 20, NotVaccinated: 2}},
		// A repeated record of the same predio and cycle counts once.
		{Code: "A", Year: 2026, Cycle: 1, Herd: model.Herd{MalesOver3Years: 20, NotVaccinated: 2}},
		{Code: "B", Year: 2026, Cycle: 1, Herd: model.Herd{Females3To5Years: 15}},
		// A predio last vaccinated in an older cycle is out.
		{Code: "C", Year: 2025, Cycle: 2, Herd: model.Herd{Females3To5Years: 400}},
	}}

	herd, found := r.CycleHerd(model.VaccinationCycle{Year: 2026, Cycle: 1})
	assert.True(t, found)
	c := herd.Classify()
	assert.Equal(t, 35, c.Total)
	assert.Equal(t, 2, c.NotVaccinated)

	_, found = r.CycleHerd(model.VaccinationCycle{Year: 2026, Cycle: 2})
	assert.False(t, found)
}
