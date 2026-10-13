package model

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// Fedegán yield data, consolidated April 2026, indexed by typology and
// department. A fattener in Guaviare gains 810 g/day and one in Nariño 630, and
// a specialized dairy cow in Antioquia gives 15.6 L/day against 13.0 in La
// Guajira, so the department materially changes the projected income.
//
// Prices per kilo and per litre are still national: Fedegán holds them per
// municipality and they get added here with the same shape when delivered.

// LivestockYields holds the per-department production tables.
type LivestockYields struct {
	// LitresPerCowDay is milk yield, by typology and department.
	LitresPerCowDay map[LivestockTypology]map[string]decimal.Decimal
	// DailyGainGrams is weight gain in grams/day, by typology and department.
	DailyGainGrams map[LivestockTypology]map[string]decimal.Decimal
}

func yieldRow(pairs ...string) map[string]decimal.Decimal {
	if len(pairs)%2 != 0 {
		panic("livestock: yield row needs department/value pairs")
	}
	row := make(map[string]decimal.Decimal, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		d, err := decimal.NewFromString(pairs[i+1])
		if err != nil {
			panic("livestock: invalid yield " + pairs[i+1])
		}
		row[pairs[i]] = d
	}
	return row
}

// DefaultLivestockYields returns the shipped Fedegán consolidation.
func DefaultLivestockYields() LivestockYields {
	return LivestockYields{
		LitresPerCowDay: map[LivestockTypology]map[string]decimal.Decimal{
			TypologySpecializedDairy: yieldRow(
				"Antioquia", "15.6", "Arauca", "13.9", "Atlántico", "13.5", "Bolívar", "13.8",
				"Boyacá", "15.2", "Casanare", "14.0", "Caldas", "15.0", "Cauca", "14.3",
				"Caquetá", "10.0", "César", "14.1", "Córdoba", "14.2", "Cundinamarca", "15.3",
				"Magdalena", "13.7", "Huila", "14.9", "Meta", "14.1", "Guaviare", "13.5",
				"Guajira", "13.0", "Nariño", "15.5", "Quindío", "15.3", "Risaralda", "15.1",
				"Santander", "15.4", "Norte de Santander", "14.2", "Sucre", "13.9",
				"Tolima", "14.8", "Valle del Cauca", "15.6", "Vichada", "13.2",
			),
			TypologyDualPurpose: yieldRow(
				"Antioquia", "7.8", "Arauca", "6.9", "Atlántico", "6.5", "Bolívar", "6.8",
				"Boyacá", "7.4", "Casanare", "7.0", "Caldas", "7.6", "Cauca", "7.2",
				"Caquetá", "5.0", "César", "7.0", "Córdoba", "7.0", "Cundinamarca", "7.6",
				"Magdalena", "6.7", "Huila", "7.5", "Meta", "7.1", "Guaviare", "6.8",
				"Guajira", "6.3", "Nariño", "7.9", "Quindío", "7.7", "Risaralda", "7.5",
				"Santander", "7.8", "Norte de Santander", "7.1", "Sucre", "6.8",
				"Tolima", "7.3", "Valle del Cauca", "7.9", "Vichada", "6.4",
			),
			TypologyCowCalf: yieldRow(
				"Antioquia", "4.0", "Arauca", "3.7", "Atlántico", "3.3", "Bolívar", "3.6",
				"Boyacá", "4.1", "Casanare", "3.5", "Caldas", "4.2", "Cauca", "3.8",
				"Caquetá", "3.0", "César", "3.5", "Córdoba", "3.6", "Cundinamarca", "4.0",
				"Magdalena", "3.6", "Huila", "3.9", "Meta", "3.5", "Guaviare", "3.4",
				"Guajira", "3.3", "Nariño", "4.0", "Quindío", "4.1", "Risaralda", "4.0",
				"Santander", "4.1", "Norte de Santander", "3.6", "Sucre", "3.4",
				"Tolima", "3.9", "Valle del Cauca", "4.1", "Vichada", "3.2",
			),
		},
		DailyGainGrams: map[LivestockTypology]map[string]decimal.Decimal{
			TypologyDualPurpose: yieldRow(
				"Antioquia", "550", "Arauca", "630", "Atlántico", "580", "Bolívar", "600",
				"Boyacá", "540", "Casanare", "640", "Caldas", "560", "Cauca", "570",
				"Caquetá", "550", "César", "610", "Córdoba", "590", "Cundinamarca", "560",
				"Magdalena", "610", "Huila", "550", "Meta", "600", "Guaviare", "620",
				"Guajira", "660", "Nariño", "530", "Quindío", "540", "Risaralda", "550",
				"Santander", "560", "Norte de Santander", "600", "Sucre", "590",
				"Tolima", "560", "Valle del Cauca", "580", "Vichada", "650",
			),
			TypologyCowCalf: yieldRow(
				"Antioquia", "470", "Arauca", "520", "Atlántico", "490", "Bolívar", "500",
				"Boyacá", "460", "Casanare", "530", "Caldas", "480", "Cauca", "485",
				"Caquetá", "500", "César", "510", "Córdoba", "500", "Cundinamarca", "475",
				"Magdalena", "500", "Huila", "470", "Meta", "510", "Guaviare", "520",
				"Guajira", "540", "Nariño", "450", "Quindío", "460", "Risaralda", "470",
				"Santander", "475", "Norte de Santander", "505", "Sucre", "500",
				"Tolima", "480", "Valle del Cauca", "490", "Vichada", "540",
			),
			TypologyFattening: yieldRow(
				"Antioquia", "650", "Arauca", "780", "Atlántico", "700", "Bolívar", "720",
				"Boyacá", "640", "Casanare", "790", "Caldas", "660", "Cauca", "670",
				"Caquetá", "800", "César", "750", "Córdoba", "720", "Cundinamarca", "670",
				"Magdalena", "740", "Huila", "660", "Meta", "770", "Guaviare", "780",
				"Guajira", "810", "Nariño", "630", "Quindío", "640", "Risaralda", "650",
				"Santander", "660", "Norte de Santander", "740", "Sucre", "730",
				"Tolima", "660", "Valle del Cauca", "700", "Vichada", "800",
			),
		},
	}
}

// litresPerCowDay looks up milk yield, falling back to the typology's national
// average when the department is absent, then to dual purpose, then to 6 L.
func (y LivestockYields) litresPerCowDay(typology LivestockTypology, department string) decimal.Decimal {
	if v, ok := lookupYield(y.LitresPerCowDay, typology, department); ok {
		return v
	}
	if v, ok := lookupYield(y.LitresPerCowDay, TypologyDualPurpose, department); ok {
		return v
	}
	return decimal.NewFromInt(6)
}

// DailyGain returns weight gain in grams/day for a typology and department, and
// whether the lookup found data.
//
// NOTE: the reference engine computes this and then never applies it to income,
// so today it does not move the quota. Porting it faithfully keeps the
// verification table reproducible; wiring it into the fattening and growing
// income is a deliberate model change and needs Fedegán's sign-off.
func (y LivestockYields) DailyGain(typology LivestockTypology, department string) (decimal.Decimal, bool) {
	return lookupYield(y.DailyGainGrams, typology, department)
}

func lookupYield(table map[LivestockTypology]map[string]decimal.Decimal, typology LivestockTypology, department string) (decimal.Decimal, bool) {
	row, ok := table[typology]
	if !ok {
		return decimal.Zero, false
	}
	if v, ok := row[department]; ok {
		return v, true
	}
	// Registries spell departments their own way ("CESAR", "LA GUAJIRA"):
	// match ignoring case, accents and a leading "La".
	want := normalizeDepartment(department)
	for name, v := range row {
		if normalizeDepartment(name) == want {
			return v, true
		}
	}
	if len(row) == 0 {
		return decimal.Zero, false
	}
	// National average when the department is not in the table.
	sum := decimal.Zero
	for _, v := range row {
		sum = sum.Add(v)
	}
	return sum.Div(decimal.NewFromInt(int64(len(row)))), true
}

// LabourRegime decides how hired labour is charged to the proforma.
type LabourRegime string

const (
	// LabourAuto scales the regime by herd size.
	LabourAuto LabourRegime = "auto"
	// LabourFamily charges nothing: the family works the holding.
	LabourFamily LabourRegime = "familiar"
	// LabourFormal charges full-time hands.
	LabourFormal LabourRegime = "formal"
)

// QuotaParams is every assumption the quota engine takes. Amounts are COP.
//
// Credit policy values come from the pilot committee (September 2026): 26% E.A.
// fixed against Fedegán capital at 12%, three months with principal at
// maturity, inputs only, and a COP 3,000,000 policy cap.
type QuotaParams struct {
	// Sale prices per kilo and reference live weights per category.
	PriceCalfPerKg      decimal.Decimal
	PriceGrowingPerKg   decimal.Decimal
	PriceFatteningPerKg decimal.Decimal
	WeightCalfKg        decimal.Decimal
	WeightGrowingKg     decimal.Decimal
	WeightFatteningKg   decimal.Decimal

	// MilkingShare is the fraction of breeding females in lactation.
	MilkingShare  decimal.Decimal
	PricePerLitre decimal.Decimal

	// Annual operating costs per UGG.
	VaccineCostPerUGGYear    decimal.Decimal
	SaltCostPerUGGYear       decimal.Decimal
	SupplementCostPerUGGYear decimal.Decimal
	VetCostPerUGGYear        decimal.Decimal
	DailyWage                decimal.Decimal

	// Labour regime and the UGG thresholds that switch it.
	LabourRegime           LabourRegime
	UGGThresholdFamily     decimal.Decimal
	UGGThresholdOccasional decimal.Decimal
	UGGThresholdPartial    decimal.Decimal
	OccasionalDaysPerMonth int

	// Upkeep: fencing per hectare, infrastructure per UGG.
	FenceCostPerHaYear  decimal.Decimal
	InfraCostPerUGGYear decimal.Decimal
	HectaresPerUGG      decimal.Decimal

	// Credit policy.
	LTVPercent        decimal.Decimal
	AnnualRatePct     decimal.Decimal
	CapitalCostPct    decimal.Decimal
	MaxQuota          decimal.Decimal
	TermMonths        int
	ApplyInputCeiling bool
	InputHeadroom     decimal.Decimal

	// UGGFactors is the typology x category conversion matrix. An empty map
	// means the shipped DefaultUGGFactors, so a QuotaParams built without it
	// behaves exactly as it did before the matrix became editable.
	UGGFactors map[LivestockTypology]UGGFactors
}

// FactorsFor returns the UGG factors a typology converts with, falling back to
// the shipped matrix for a typology the parameters do not cover.
func (p QuotaParams) FactorsFor(typology LivestockTypology) UGGFactors {
	if f, ok := p.UGGFactors[typology]; ok {
		return f
	}
	return FactorsFor(typology)
}

// DefaultQuotaParams returns the CuppoGan pilot parameters.
func DefaultQuotaParams() QuotaParams {
	d := decimal.NewFromInt
	f := decimal.NewFromFloat
	return QuotaParams{
		PriceCalfPerKg:      d(7000),
		PriceGrowingPerKg:   d(8000),
		PriceFatteningPerKg: d(9000),
		WeightCalfKg:        d(180),
		WeightGrowingKg:     d(320),
		WeightFatteningKg:   d(450),

		MilkingShare:  f(0.30),
		PricePerLitre: d(1800),

		VaccineCostPerUGGYear:    d(18000),
		SaltCostPerUGGYear:       d(35000),
		SupplementCostPerUGGYear: d(80000),
		VetCostPerUGGYear:        d(25000),
		DailyWage:                d(55000),

		LabourRegime:           LabourAuto,
		UGGThresholdFamily:     d(30),
		UGGThresholdOccasional: d(80),
		UGGThresholdPartial:    d(200),
		OccasionalDaysPerMonth: 4,

		FenceCostPerHaYear:  d(45000),
		InfraCostPerUGGYear: d(30000),
		HectaresPerUGG:      f(1.2),

		LTVPercent:        d(30),
		AnnualRatePct:     d(26),
		CapitalCostPct:    d(12),
		MaxQuota:          d(3000000),
		TermMonths:        3,
		ApplyInputCeiling: true,
		InputHeadroom:     f(1.15),

		UGGFactors: DefaultUGGFactors(),
	}
}

// periodDays is the projection window, aligned to the credit term.
func (p QuotaParams) periodDays() decimal.Decimal {
	return decimal.NewFromInt(int64(p.termMonths() * 30))
}

func (p QuotaParams) termMonths() int {
	if p.TermMonths <= 0 {
		return 3
	}
	return p.TermMonths
}

// yearFraction is the share of a year the term covers (3 months = 0.25).
func (p QuotaParams) yearFraction() decimal.Decimal {
	return decimal.NewFromInt(int64(p.termMonths())).Div(decimal.NewFromInt(12))
}

// rotationScale rescales the engine's semi-annual inventory rotations to the
// configured term.
func (p QuotaParams) rotationScale() decimal.Decimal {
	return p.periodDays().Div(decimal.NewFromInt(180))
}

// normalizeDepartment folds a department name for lookup: "LA GUAJIRA",
// "Guajira" and "guajira" are the same, as are "CÓRDOBA" and "Cordoba".
func normalizeDepartment(name string) string {
	folded := strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'Á':
			return 'a'
		case 'é', 'É':
			return 'e'
		case 'í', 'Í':
			return 'i'
		case 'ó', 'Ó':
			return 'o'
		case 'ú', 'Ú', 'ü', 'Ü':
			return 'u'
		}
		return unicode.ToLower(r)
	}, strings.Join(strings.Fields(name), " "))
	return strings.TrimPrefix(folded, "la ")
}
