package model

import "github.com/shopspring/decimal"

// This file ports the herd-classification half of the CuppoGan quota engine:
// the reference implementation is an executable HTML prototype, and its README
// fixes a verification table of ten holdings that this port must reproduce.
//
// A holding's creditworthiness is inferred from Fedegán's RUV (Registro Único
// de Vacunación): the foot-and-mouth vaccination registry reports how many
// animals a national ID has, by age and sex, twice a year. For ranchers with no
// bureau history it is the only observable signal of productive capacity.

// LivestockTypology is the productive orientation inferred from herd
// composition. It selects both the UGG factors and the yield tables.
type LivestockTypology string

const (
	TypologySpecializedDairy LivestockTypology = "Lechería Especializada"
	TypologyDualPurpose      LivestockTypology = "Doble Propósito"
	TypologyCowCalf          LivestockTypology = "Cría"
	TypologyFattening        LivestockTypology = "Ceba"
)

// Herd is one holding's animal count as reported by the RUV, already mapped
// from the registry's field names. Counts are vaccinated animals except
// NotVaccinated, which is the registry's unvaccinated total for the holding.
//
// CalvesUnder1Year has no RUV equivalent: animals from 0 to 3 months already
// arrive in FemalesUnder3Months / MalesUnder3Months. It exists because the
// reference prototype was built on synthetic data, and the RUV mapper leaves it
// at zero.
type Herd struct {
	CalvesUnder1Year int

	FemalesUnder3Months int
	Females3To8Months   int
	Females8To12Months  int
	Females1To2Years    int
	Females2To3Years    int
	Females3To5Years    int
	FemalesOver5Years   int

	MalesUnder3Months int
	Males3To8Months   int
	Males8To12Months  int
	Males1To2Years    int
	Males2To3Years    int
	MalesOver3Years   int

	NotVaccinated int
}

// Add accumulates another holding's animals into this herd. A single national
// ID can hold several predios in the RUV, and the quota is one per ID, so the
// herds are summed before classification.
func (h *Herd) Add(other Herd) {
	h.CalvesUnder1Year += other.CalvesUnder1Year
	h.FemalesUnder3Months += other.FemalesUnder3Months
	h.Females3To8Months += other.Females3To8Months
	h.Females8To12Months += other.Females8To12Months
	h.Females1To2Years += other.Females1To2Years
	h.Females2To3Years += other.Females2To3Years
	h.Females3To5Years += other.Females3To5Years
	h.FemalesOver5Years += other.FemalesOver5Years
	h.MalesUnder3Months += other.MalesUnder3Months
	h.Males3To8Months += other.Males3To8Months
	h.Males8To12Months += other.Males8To12Months
	h.Males1To2Years += other.Males1To2Years
	h.Males2To3Years += other.Males2To3Years
	h.MalesOver3Years += other.MalesOver3Years
	h.NotVaccinated += other.NotVaccinated
}

// HerdClassification collapses the RUV's age/sex columns into the five
// commercial categories the engine prices.
type HerdClassification struct {
	// Calves are animals under one year, of either sex.
	Calves int
	// Growing are yearlings, 1 to 2 years, of either sex.
	Growing int
	// Fattening are males from 2 years up: the ones sold to the abattoir.
	Fattening int
	// Breeding are females from 2 to 5 years: the ones that calve and milk.
	Breeding int
	// Cull are females over 5 years.
	Cull int

	Total         int
	NotVaccinated int
}

// Classify collapses the herd into commercial categories.
func (h Herd) Classify() HerdClassification {
	calves := h.CalvesUnder1Year +
		h.FemalesUnder3Months + h.Females3To8Months + h.Females8To12Months +
		h.MalesUnder3Months + h.Males3To8Months + h.Males8To12Months
	growing := h.Females1To2Years + h.Males1To2Years
	fattening := h.Males2To3Years + h.MalesOver3Years
	breeding := h.Females2To3Years + h.Females3To5Years
	cull := h.FemalesOver5Years

	return HerdClassification{
		Calves:        calves,
		Growing:       growing,
		Fattening:     fattening,
		Breeding:      breeding,
		Cull:          cull,
		Total:         calves + growing + fattening + breeding + cull,
		NotVaccinated: h.NotVaccinated,
	}
}

// Typology infers the productive orientation from herd composition.
//
// The thresholds are calibrated from composition analysis, not supplied by
// Fedegán; if Fedegán delivers its own segmentation this is the single place to
// replace. Dual purpose is the default and the majority case in Colombia.
func (c HerdClassification) Typology() LivestockTypology {
	if c.Total == 0 {
		return TypologyDualPurpose
	}
	total := decimal.NewFromInt(int64(c.Total))
	pct := func(n int) decimal.Decimal {
		return decimal.NewFromInt(int64(n)).Div(total)
	}

	breeding := pct(c.Breeding)
	fattening := pct(c.Fattening)
	rearing := pct(c.Calves + c.Growing)

	switch {
	case breeding.GreaterThan(decimal.NewFromFloat(0.50)) && fattening.LessThan(decimal.NewFromFloat(0.10)):
		return TypologySpecializedDairy
	case fattening.GreaterThan(decimal.NewFromFloat(0.40)):
		return TypologyFattening
	case rearing.GreaterThan(decimal.NewFromFloat(0.55)) && breeding.GreaterThan(decimal.NewFromFloat(0.15)):
		return TypologyCowCalf
	default:
		return TypologyDualPurpose
	}
}

// UGGFactors converts each commercial category into Unidad Gran Ganado, the
// common equivalent used to scale costs. Reference: an adult third-calving dual
// purpose cow is 1.0 UGG.
//
// The factors vary by typology because a specialized-dairy Holstein weighs and
// eats more than a cow-calf Cebú. These values are a technical estimate and are
// NOT validated by Fedegán: they live in one table so the official matrix can
// replace them wholesale.
type UGGFactors struct {
	Calves    decimal.Decimal
	Growing   decimal.Decimal
	Breeding  decimal.Decimal
	Fattening decimal.Decimal
	Cull      decimal.Decimal
}

func uggFactor(calves, growing, breeding, fattening, cull string) UGGFactors {
	must := func(s string) decimal.Decimal {
		d, err := decimal.NewFromString(s)
		if err != nil {
			panic("livestock: invalid UGG factor " + s)
		}
		return d
	}
	return UGGFactors{
		Calves:    must(calves),
		Growing:   must(growing),
		Breeding:  must(breeding),
		Fattening: must(fattening),
		Cull:      must(cull),
	}
}

// DefaultUGGFactors is the 4 typologies x 5 categories matrix.
func DefaultUGGFactors() map[LivestockTypology]UGGFactors {
	return map[LivestockTypology]UGGFactors{
		//                              calves growing breeding fattening cull
		TypologySpecializedDairy: uggFactor("0.30", "0.65", "1.20", "0.00", "1.10"),
		TypologyDualPurpose:      uggFactor("0.25", "0.55", "1.00", "1.10", "1.00"),
		TypologyCowCalf:          uggFactor("0.25", "0.55", "0.90", "1.00", "0.95"),
		TypologyFattening:        uggFactor("0.30", "0.70", "0.90", "1.20", "0.95"),
	}
}

// FactorsFor returns the UGG factors for a typology, falling back to dual
// purpose for an unknown one.
func FactorsFor(typology LivestockTypology) UGGFactors {
	factors := DefaultUGGFactors()
	if f, ok := factors[typology]; ok {
		return f
	}
	return factors[TypologyDualPurpose]
}

// UGGLine is one category's contribution to the herd's UGG, kept for the
// traceability the risk team asks for.
type UGGLine struct {
	Category string
	Head     int
	Factor   decimal.Decimal
	UGG      decimal.Decimal
}

// UGG converts the herd to Unidad Gran Ganado under the given typology, using
// the shipped factor matrix.
func (c HerdClassification) UGG(typology LivestockTypology) decimal.Decimal {
	return c.UGGWith(FactorsFor(typology))
}

// UGGWith converts the herd using an explicit factor row, which is how the
// engine applies an operator-edited matrix.
func (c HerdClassification) UGGWith(f UGGFactors) decimal.Decimal {
	total := decimal.Zero
	for _, line := range c.UGGBreakdownWith(f) {
		total = total.Add(line.UGG)
	}
	return total
}

// UGGBreakdown itemizes the UGG conversion per category.
func (c HerdClassification) UGGBreakdown(typology LivestockTypology) []UGGLine {
	return c.UGGBreakdownWith(FactorsFor(typology))
}

// UGGBreakdownWith itemizes the conversion under an explicit factor row.
func (c HerdClassification) UGGBreakdownWith(f UGGFactors) []UGGLine {
	line := func(category string, head int, factor decimal.Decimal) UGGLine {
		return UGGLine{
			Category: category,
			Head:     head,
			Factor:   factor,
			UGG:      decimal.NewFromInt(int64(head)).Mul(factor),
		}
	}
	return []UGGLine{
		line("Calves", c.Calves, f.Calves),
		line("Growing", c.Growing, f.Growing),
		line("Fattening", c.Fattening, f.Fattening),
		line("Breeding", c.Breeding, f.Breeding),
		line("Cull", c.Cull, f.Cull),
	}
}
