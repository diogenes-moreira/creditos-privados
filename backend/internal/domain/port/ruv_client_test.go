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

func TestOwnsAHoldingReadsPropietario(t *testing.T) {
	rancher := func(owners ...string) port.RUVRancher {
		r := port.RUVRancher{NationalID: "17788090", Name: "JOSÉ  PÉREZ GÓMEZ"}
		for i, o := range owners {
			r.Holdings = append(r.Holdings, port.RUVHolding{
				Code: string(rune('A' + i)), Owner: o,
				Registrant: "JOSÉ PÉREZ GÓMEZ", RegistrantID: "17788090",
			})
		}
		return r
	}
	for owner, want := range map[string]bool{
		"si": true, "Sí": true, " SI ": true, "TRUE": true, "true": true,
		"jose perez  gomez": true, "José Pérez Gómez": true, " JOSE PEREZ GOMEZ ": true,
		"17.788.090": true, "17788090": true, "17 788 090": true,
		"MARÍA PÉREZ": false, "no": false, "false": false, "": false,
		"12345678": false, "Finca 17788090": false, "arrendatario": false,
	} {
		assert.Equal(t, want, rancher(owner).OwnsAHolding(), "propietario %q", owner)
	}

	assert.False(t, rancher().OwnsAHolding(), "no records")
	assert.False(t, rancher("", "no", "OTRO SEÑOR").OwnsAHolding())
	assert.True(t, rancher("", "no", "sí").OwnsAHolding(), "one owned predio is enough")

	// Without per-record identity, the rancher's own name and ID are used.
	bare := port.RUVRancher{NationalID: "17788090", Name: "José Pérez", Holdings: []port.RUVHolding{{Owner: "JOSE PEREZ"}}}
	assert.True(t, bare.OwnsAHolding())
	bare.Holdings[0].Owner = "17.788.090"
	assert.True(t, bare.OwnsAHolding())
}
