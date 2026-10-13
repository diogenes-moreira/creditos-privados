package model_test

import (
	"fmt"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reference engine ships ten synthetic holdings and the exact figures they
// must produce: "si portás el algoritmo y no te dan estos números, algo
// cambió". This is that acceptance test.

type goldenHolding struct {
	id         string
	department string
	tenure     string
	status     string
	seniority  int
	completed  int
	reported   int
	herd       model.Herd

	wantTypology model.LivestockTypology
	wantHead     int
	wantUGG      int
	wantScore    int
	// wantQuota is the COP figure the reference README publishes, and
	// wantTolerance is half of its last significant digit: the table prints
	// "$711K" (+/-500) and "$1.27M" (+/-5000), so the quota is compared at the
	// precision it was actually published with.
	wantQuota     string
	wantTolerance string
	wantCeiling   string
}

func goldenHoldings() []goldenHolding {
	return []goldenHolding{
		{
			id: "001", department: "Córdoba", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 12, completed: 12, reported: 12,
			herd: model.Herd{
				FemalesUnder3Months: 1, Females3To8Months: 1, Females8To12Months: 0,
				Females1To2Years: 1, Females2To3Years: 1, Females3To5Years: 1, FemalesOver5Years: 1,
				CalvesUnder1Year:  2,
				MalesUnder3Months: 0, Males3To8Months: 0, Males8To12Months: 0,
				Males1To2Years: 2, Males2To3Years: 5, MalesOver3Years: 3,
				NotVaccinated: 1,
			},
			wantTypology: model.TypologyFattening, wantHead: 18, wantUGG: 16, wantScore: 90,
			wantQuota: "711000", wantTolerance: "500", wantCeiling: model.CeilingInputs,
		},
		{
			id: "002", department: "César", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 18, completed: 18, reported: 18,
			herd: model.Herd{
				FemalesUnder3Months: 1, Females3To8Months: 1, Females8To12Months: 1,
				Females1To2Years: 2, Females2To3Years: 4, Females3To5Years: 7, FemalesOver5Years: 2,
				CalvesUnder1Year: 2,
				NotVaccinated:    0,
			},
			wantTypology: model.TypologySpecializedDairy, wantHead: 20, wantUGG: 18, wantScore: 98,
			wantQuota: "827000", wantTolerance: "500", wantCeiling: model.CeilingInputs,
		},
		{
			id: "003", department: "Casanare", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 8, completed: 6, reported: 8,
			herd: model.Herd{
				FemalesUnder3Months: 3, Females3To8Months: 4, Females8To12Months: 5,
				Females1To2Years: 8, Females2To3Years: 7, Females3To5Years: 8, FemalesOver5Years: 8,
				CalvesUnder1Year:  8,
				MalesUnder3Months: 2, Males3To8Months: 3, Males8To12Months: 0,
				Males1To2Years: 12, Males2To3Years: 35, MalesOver3Years: 25,
				NotVaccinated: 14,
			},
			wantTypology: model.TypologyFattening, wantHead: 128, wantUGG: 115, wantScore: 77,
			wantQuota: "3000000", wantTolerance: "0", wantCeiling: model.CeilingPolicyCap,
		},
		{
			id: "004", department: "Sucre", tenure: model.TenureLeased, status: model.HoldingActive,
			seniority: 4, completed: 4, reported: 4,
			herd: model.Herd{
				FemalesUnder3Months: 1, Females3To8Months: 1, Females8To12Months: 0,
				Females1To2Years: 2, Females2To3Years: 2, Females3To5Years: 2, FemalesOver5Years: 1,
				CalvesUnder1Year: 2,
				Males1To2Years:   1, Males2To3Years: 1, MalesOver3Years: 1,
				NotVaccinated: 1,
			},
			wantTypology: model.TypologyDualPurpose, wantHead: 14, wantUGG: 10, wantScore: 79,
			wantQuota: "330000", wantTolerance: "500", wantCeiling: model.CeilingLTV,
		},
		{
			id: "005", department: "Antioquia", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 22, completed: 22, reported: 22,
			herd: model.Herd{
				FemalesUnder3Months: 2, Females3To8Months: 3, Females8To12Months: 2,
				Females1To2Years: 4, Females2To3Years: 4, Females3To5Years: 6, FemalesOver5Years: 4,
				CalvesUnder1Year:  4,
				MalesUnder3Months: 1, Males1To2Years: 4, Males2To3Years: 4, MalesOver3Years: 2,
				NotVaccinated: 1,
			},
			wantTypology: model.TypologyDualPurpose, wantHead: 40, wantUGG: 28, wantScore: 99,
			wantQuota: "1270000", wantTolerance: "5000", wantCeiling: model.CeilingInputs,
		},
		{
			id: "006", department: "Magdalena", tenure: model.TenureLeased, status: model.HoldingActive,
			seniority: 3, completed: 2, reported: 3,
			herd: model.Herd{
				FemalesUnder3Months: 2, Females3To8Months: 2, Females8To12Months: 2,
				Females1To2Years: 4, Females2To3Years: 3, Females3To5Years: 3, FemalesOver5Years: 2,
				CalvesUnder1Year:  4,
				MalesUnder3Months: 1, Males3To8Months: 1, Males1To2Years: 4, Males2To3Years: 2,
				NotVaccinated: 6,
			},
			wantTypology: model.TypologyCowCalf, wantHead: 30, wantUGG: 17, wantScore: 64,
			wantQuota: "759000", wantTolerance: "500", wantCeiling: model.CeilingInputs,
		},
		{
			id: "007", department: "Boyacá", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 14, completed: 14, reported: 14,
			herd: model.Herd{
				FemalesUnder3Months: 1, Females3To8Months: 1, Females8To12Months: 0,
				Females1To2Years: 2, Females2To3Years: 3, Females3To5Years: 6, FemalesOver5Years: 2,
				CalvesUnder1Year: 2,
				NotVaccinated:    0,
			},
			wantTypology: model.TypologySpecializedDairy, wantHead: 17, wantUGG: 16, wantScore: 94,
			wantQuota: "704000", wantTolerance: "500", wantCeiling: model.CeilingInputs,
		},
		{
			id: "008", department: "Meta", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 25, completed: 25, reported: 25,
			herd: model.Herd{
				FemalesUnder3Months: 15, Females3To8Months: 22, Females8To12Months: 28,
				Females1To2Years: 45, Females2To3Years: 35, Females3To5Years: 45, FemalesOver5Years: 30,
				CalvesUnder1Year:  40,
				MalesUnder3Months: 8, Males3To8Months: 7, Males8To12Months: 0,
				Males1To2Years: 45, Males2To3Years: 50, MalesOver3Years: 30,
				NotVaccinated: 8,
			},
			wantTypology: model.TypologyDualPurpose, wantHead: 400, wantUGG: 278, wantScore: 99,
			wantQuota: "3000000", wantTolerance: "0", wantCeiling: model.CeilingPolicyCap,
		},
		{
			id: "009", department: "Bolívar", tenure: model.TenureLeased, status: model.HoldingActive,
			seniority: 6, completed: 4, reported: 6,
			herd: model.Herd{
				FemalesUnder3Months: 3, Females3To8Months: 4, Females8To12Months: 3,
				Females1To2Years: 7, Females2To3Years: 6, Females3To5Years: 8, FemalesOver5Years: 5,
				CalvesUnder1Year:  8,
				MalesUnder3Months: 1, Males3To8Months: 1, Males1To2Years: 7,
				Males2To3Years: 6, MalesOver3Years: 4,
				NotVaccinated: 4,
			},
			wantTypology: model.TypologyDualPurpose, wantHead: 63, wantUGG: 43, wantScore: 71,
			wantQuota: "1940000", wantTolerance: "5000", wantCeiling: model.CeilingInputs,
		},
		{
			id: "010", department: "Caquetá", tenure: model.TenureOwned, status: model.HoldingActive,
			seniority: 9, completed: 9, reported: 9,
			herd: model.Herd{
				FemalesUnder3Months: 1, Females3To8Months: 1, Females8To12Months: 1,
				Females1To2Years: 2, Females2To3Years: 2, Females3To5Years: 2, FemalesOver5Years: 1,
				CalvesUnder1Year: 2,
				Males1To2Years:   1, Males2To3Years: 2, MalesOver3Years: 1,
				NotVaccinated: 0,
			},
			wantTypology: model.TypologyDualPurpose, wantHead: 16, wantUGG: 11, wantScore: 89,
			wantQuota: "267000", wantTolerance: "500", wantCeiling: model.CeilingLTV,
		},
	}
}

func (g goldenHolding) applicant() model.LivestockApplicant {
	return model.LivestockApplicant{
		NationalID:      g.id,
		Department:      g.department,
		Herd:            g.herd,
		Tenure:          g.tenure,
		HoldingStatus:   g.status,
		SeniorityCycles: g.seniority,
		CompletedCycles: g.completed,
		ReportedCycles:  g.reported,
	}
}

func TestQuotaEngineReproducesReferenceTable(t *testing.T) {
	params := model.DefaultQuotaParams()
	yields := model.DefaultLivestockYields()

	for _, g := range goldenHoldings() {
		t.Run("cc"+g.id, func(t *testing.T) {
			got := model.AssessQuota(g.applicant(), params, yields)

			assert.Equal(t, g.wantTypology, got.Typology, "typology")
			assert.Equal(t, g.wantHead, got.Classification.Total, "head count")
			assert.Equal(t, g.wantUGG, int(got.UGG.Round(0).IntPart()), "UGG (got %s)", got.UGG)
			assert.Equal(t, g.wantScore, got.Score.Score, "behaviour score")
			assert.Equal(t, g.wantCeiling, got.Quota.ActiveCeiling, "active ceiling")

			want, err := decimal.NewFromString(g.wantQuota)
			require.NoError(t, err)
			tolerance, err := decimal.NewFromString(g.wantTolerance)
			require.NoError(t, err)

			drift := got.Quota.Quota.Sub(want).Abs()
			assert.True(t, drift.LessThanOrEqual(tolerance),
				"quota: published %s COP (+/-%s), got %s COP, off by %s",
				want, tolerance, got.Quota.Quota.StringFixed(0), drift.StringFixed(0))
		})
	}
}

func TestQuotaEnginePilotProfile(t *testing.T) {
	params := model.DefaultQuotaParams()
	yields := model.DefaultLivestockYields()

	// The README's headline check: six of the ten land under a million COP,
	// which is the pilot's target profile.
	underMillion := 0
	for _, g := range goldenHoldings() {
		got := model.AssessQuota(g.applicant(), params, yields)
		if got.Quota.Quota.LessThan(decimal.NewFromInt(1000000)) {
			underMillion++
		}
	}
	assert.Equal(t, 6, underMillion, "expected six holdings under COP 1MM")
}

func TestHerdAddAggregatesMultiplePredios(t *testing.T) {
	// One national ID can hold several predios and the quota is one per ID, so
	// the herds must be summed before classifying.
	a := model.Herd{Females2To3Years: 3, Males2To3Years: 2, NotVaccinated: 1}
	a.Add(model.Herd{Females2To3Years: 2, Males2To3Years: 1, NotVaccinated: 4})

	assert.Equal(t, 5, a.Females2To3Years)
	assert.Equal(t, 3, a.Males2To3Years)
	assert.Equal(t, 5, a.NotVaccinated)

	c := a.Classify()
	assert.Equal(t, 8, c.Total)
	assert.Equal(t, 5, c.Breeding)
	assert.Equal(t, 3, c.Fattening)
}

func TestClassifyEmptyHerd(t *testing.T) {
	c := model.Herd{}.Classify()

	assert.Equal(t, 0, c.Total)
	// An empty herd must not panic and defaults to the majority typology.
	assert.Equal(t, model.TypologyDualPurpose, c.Typology())
	assert.True(t, c.UGG(model.TypologyDualPurpose).IsZero())
}

func TestTypologyThresholds(t *testing.T) {
	tests := []struct {
		name string
		herd model.Herd
		want model.LivestockTypology
	}{
		{
			name: "breeding over half with almost no fattening is dairy",
			herd: model.Herd{Females2To3Years: 6, Females3To5Years: 3, Females1To2Years: 1},
			want: model.TypologySpecializedDairy,
		},
		{
			name: "fattening over 40 percent is fattening",
			herd: model.Herd{Males2To3Years: 5, MalesOver3Years: 3, Females2To3Years: 2},
			want: model.TypologyFattening,
		},
		{
			name: "mostly young stock with some breeders is cow-calf",
			herd: model.Herd{CalvesUnder1Year: 6, Females1To2Years: 3, Females2To3Years: 2},
			want: model.TypologyCowCalf,
		},
		{
			name: "mixed herd falls through to dual purpose",
			herd: model.Herd{Females2To3Years: 3, Males2To3Years: 2, Females1To2Years: 3, CalvesUnder1Year: 2},
			want: model.TypologyDualPurpose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.herd.Classify().Typology())
		})
	}
}

func TestBehaviourScoreBounds(t *testing.T) {
	// A perfect record: everything vaccinated, every cycle complete, long
	// seniority, owned and active.
	best := model.LivestockApplicant{
		Herd:            model.Herd{Females2To3Years: 10},
		Tenure:          model.TenureOwned,
		HoldingStatus:   model.HoldingActive,
		SeniorityCycles: 25, CompletedCycles: 10, ReportedCycles: 10,
	}
	perfect := best.BehaviourScore(best.Herd.Classify())
	assert.Equal(t, 100, perfect.Score)
	assert.Equal(t, "AAA", perfect.Grade())
	assert.True(t, perfect.Multiplier().Equal(decimal.NewFromFloat(1.20)))

	// The floor: nothing vaccinated, no cycles, brand new, leased and merged.
	worst := model.LivestockApplicant{
		Herd:          model.Herd{NotVaccinated: 10},
		Tenure:        model.TenureLeased,
		HoldingStatus: "E",
	}
	low := worst.BehaviourScore(worst.Herd.Classify())
	// Nothing vaccinated and no cycles zeroes the first three components, so
	// only tenure and status score: 0.7*10 + 0.5*5 = 9.5, which rounds to 10
	// the same way the reference's Math.round does.
	assert.Equal(t, 10, low.Score)
	assert.Equal(t, "D", low.Grade())
	assert.True(t, low.Multiplier().Equal(decimal.NewFromFloat(0.50)))
}

func TestBehaviourScoreSeniralitySaturates(t *testing.T) {
	base := model.LivestockApplicant{
		Herd:            model.Herd{Females2To3Years: 5},
		Tenure:          model.TenureOwned,
		HoldingStatus:   model.HoldingActive,
		CompletedCycles: 1, ReportedCycles: 1,
	}
	at20 := base
	at20.SeniorityCycles = 20
	at40 := base
	at40.SeniorityCycles = 40

	// Seniority saturates at 20 cycles (10 years), so more does not help.
	assert.Equal(t,
		at20.BehaviourScore(at20.Herd.Classify()).Score,
		at40.BehaviourScore(at40.Herd.Classify()).Score)
}

func TestQuotaCeilingsAndTrace(t *testing.T) {
	params := model.DefaultQuotaParams()
	yields := model.DefaultLivestockYields()

	t.Run("policy cap is never exceeded", func(t *testing.T) {
		// The largest holding in the reference set.
		var big goldenHolding
		for _, g := range goldenHoldings() {
			if g.id == "008" {
				big = g
			}
		}
		got := model.AssessQuota(big.applicant(), params, yields)

		assert.True(t, got.Quota.Quota.LessThanOrEqual(params.MaxQuota))
		assert.True(t, got.Quota.CapReached)
		assert.Equal(t, model.CeilingPolicyCap, got.Quota.ActiveCeiling)
	})

	t.Run("dropping the inputs ceiling inflates the quota", func(t *testing.T) {
		// This is the ceiling the reference calls out as the one that makes the
		// model usable: without it a rancher who spends 600K on salt gets a
		// quota in the tens of millions.
		var holding goldenHolding
		for _, g := range goldenHoldings() {
			if g.id == "005" {
				holding = g
			}
		}
		withCeiling := model.AssessQuota(holding.applicant(), params, yields)

		noCeiling := params
		noCeiling.ApplyInputCeiling = false
		without := model.AssessQuota(holding.applicant(), noCeiling, yields)

		assert.Equal(t, model.CeilingInputs, withCeiling.Quota.ActiveCeiling)
		assert.True(t, without.Quota.Quota.GreaterThan(withCeiling.Quota.Quota),
			"removing the inputs ceiling must raise the quota")
		assert.NotEqual(t, model.CeilingInputs, without.Quota.ActiveCeiling)
	})

	t.Run("unit economics follow the spread", func(t *testing.T) {
		got := model.AssessQuota(goldenHoldings()[0].applicant(), params, yields)

		// 26% charged against Fedegán capital at 12% is a 14 point spread.
		assert.True(t, got.Quota.SpreadPoints.Equal(decimal.NewFromInt(14)))
		assert.True(t, got.Quota.GrossMargin.Equal(
			got.Quota.PeriodInterest.Sub(got.Quota.PeriodCapitalCost)))
		assert.True(t, got.Quota.GrossMargin.IsPositive())
		assert.Equal(t, 3, got.Quota.TermMonths)
	})

	t.Run("a zero herd yields no quota", func(t *testing.T) {
		got := model.AssessQuota(model.LivestockApplicant{Department: "Meta"}, params, yields)

		assert.True(t, got.Quota.Quota.IsZero(), "got %s", got.Quota.Quota)
	})
}

func TestProjectCostsLabourRegimeScalesWithHerd(t *testing.T) {
	params := model.DefaultQuotaParams()

	tests := []struct {
		ugg        int64
		wantRegime string
	}{
		{15, "Familiar"},
		{50, "Ocasional"},
		{120, "Parcial"},
		{300, "Formal"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d UGG", tt.ugg), func(t *testing.T) {
			costs := model.ProjectCosts(decimal.NewFromInt(tt.ugg), params)
			assert.Equal(t, tt.wantRegime, costs.AppliedRegime)

			// The eligible basket never includes wages or upkeep: they are not
			// GANEXPO catalogue purchases.
			assert.True(t, costs.EligibleBasket.Equal(
				costs.Vaccines.Add(costs.Salt).Add(costs.Supplements).Add(costs.Vet)))
			assert.True(t, costs.EligibleBasket.LessThanOrEqual(costs.Total))
		})
	}

	t.Run("family holdings are charged no wages", func(t *testing.T) {
		costs := model.ProjectCosts(decimal.NewFromInt(15), params)
		assert.True(t, costs.Wages.IsZero())
	})

	t.Run("regime can be forced", func(t *testing.T) {
		forced := params
		forced.LabourRegime = model.LabourFamily
		costs := model.ProjectCosts(decimal.NewFromInt(500), forced)

		assert.Equal(t, "Familiar", costs.AppliedRegime)
		assert.True(t, costs.Wages.IsZero())
	})
}

func TestYieldLookupFallsBackToNationalAverage(t *testing.T) {
	yields := model.DefaultLivestockYields()

	// A department Fedegán has data for.
	guajira, ok := yields.DailyGain(model.TypologyFattening, "Guajira")
	require.True(t, ok)
	assert.True(t, guajira.Equal(decimal.NewFromInt(810)))

	narino, ok := yields.DailyGain(model.TypologyFattening, "Nariño")
	require.True(t, ok)
	assert.True(t, narino.Equal(decimal.NewFromInt(630)))

	// An unknown department averages rather than failing.
	unknown, ok := yields.DailyGain(model.TypologyFattening, "Amazonas")
	require.True(t, ok)
	assert.True(t, unknown.GreaterThan(narino) && unknown.LessThan(guajira),
		"national average should sit between the extremes, got %s", unknown)
}

func TestMilkYieldDrivesDepartmentDifference(t *testing.T) {
	params := model.DefaultQuotaParams()
	yields := model.DefaultLivestockYields()

	// Same dairy herd, two departments: Antioquia at 15.6 L/day against
	// Caquetá at 10.0 must produce a materially different projection.
	herd := model.Herd{Females2To3Years: 6, Females3To5Years: 6, Females1To2Years: 1}
	c := herd.Classify()
	require.Equal(t, model.TypologySpecializedDairy, c.Typology())

	antioquia := model.ProjectIncome(c, c.Typology(), "Antioquia", params, yields)
	caqueta := model.ProjectIncome(c, c.Typology(), "Caquetá", params, yields)

	assert.True(t, antioquia.Milk.Amount.GreaterThan(caqueta.Milk.Amount))
	assert.True(t, antioquia.Total.GreaterThan(caqueta.Total))
}

func TestQuotaCeilingCodeIsStable(t *testing.T) {
	params := model.DefaultQuotaParams()
	yields := model.DefaultLivestockYields()

	// The display name can be translated or reworded; the code is what callers
	// branch on, so every holding must carry one and the two must agree.
	pairs := map[string]string{
		model.CeilingLTV:       model.CeilingCodeLTV,
		model.CeilingCapacity:  model.CeilingCodeCapacity,
		model.CeilingInputs:    model.CeilingCodeInputs,
		model.CeilingPolicyCap: model.CeilingCodePolicyCap,
	}

	seen := map[string]bool{}
	for _, g := range goldenHoldings() {
		got := model.AssessQuota(g.applicant(), params, yields)
		code := got.Quota.ActiveCeilingCode
		require.NotEmpty(t, code, "cc%s has no ceiling code", g.id)
		assert.Equal(t, pairs[got.Quota.ActiveCeiling], code,
			"cc%s: code must match the display name", g.id)
		seen[code] = true
	}

	// The reference set exercises three of the four ceilings; a code that never
	// appears would hide a mapping mistake.
	assert.True(t, seen[model.CeilingCodeInputs])
	assert.True(t, seen[model.CeilingCodeLTV])
	assert.True(t, seen[model.CeilingCodePolicyCap])
}
