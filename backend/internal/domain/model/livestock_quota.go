package model

import (
	"math"

	"github.com/shopspring/decimal"
)

// PropertyTenure and holding status are RUV attributes that feed the score.
const (
	TenureOwned  = "Propia"
	TenureLeased = "Arrendada"
	// HoldingActive is the RUV's "N" status; anything else (a merged holding)
	// scores lower.
	HoldingActive = "N"
)

// LivestockApplicant is everything the quota engine needs about a rancher:
// the aggregated herd plus the RUV behaviour history.
type LivestockApplicant struct {
	NationalID string
	Department string
	Herd       Herd

	// Tenure is TenureOwned or TenureLeased.
	Tenure string
	// HoldingStatus is the RUV's zone/holding status; HoldingActive is best.
	HoldingStatus string
	// SeniorityCycles is how many RUV cycles the holding has been registered.
	SeniorityCycles int
	// CompletedCycles / ReportedCycles is vaccination-cycle compliance.
	CompletedCycles int
	ReportedCycles  int
}

// BehaviourScore is the 0-100 alternative-bureau score and its components.
//
// Vaccination coverage carries the heaviest weight deliberately: it is the only
// operational-discipline signal available for someone with no credit history. A
// rancher who vaccinates the whole herd, every cycle, for years is
// demonstrating something.
type BehaviourScore struct {
	Score int

	VaccinationRatePct int
	CycleComplaincePct int
	SeniorityCycles    int
	Tenure             string
	HoldingStatus      string
}

// Grade buckets the score for presentation.
func (b BehaviourScore) Grade() string {
	switch {
	case b.Score >= 90:
		return "AAA"
	case b.Score >= 80:
		return "AA"
	case b.Score >= 70:
		return "A"
	case b.Score >= 60:
		return "B"
	case b.Score >= 50:
		return "C"
	default:
		return "D"
	}
}

// Multiplier is the stepped factor the score applies to the LTV ceiling.
func (b BehaviourScore) Multiplier() decimal.Decimal {
	switch {
	case b.Score >= 90:
		return decimal.NewFromFloat(1.20)
	case b.Score >= 80:
		return decimal.NewFromFloat(1.10)
	case b.Score >= 70:
		return decimal.NewFromInt(1)
	case b.Score >= 60:
		return decimal.NewFromFloat(0.85)
	case b.Score >= 50:
		return decimal.NewFromFloat(0.70)
	default:
		return decimal.NewFromFloat(0.50)
	}
}

// BehaviourScore computes the weighted score: 35% vaccination coverage,
// 30% cycle compliance, 20% RUV seniority (saturating at 20 cycles / 10 years),
// 10% tenure and 5% holding status.
func (a LivestockApplicant) BehaviourScore(c HerdClassification) BehaviourScore {
	totalAnimals := c.Total + c.NotVaccinated

	vaccinationRate := decimal.Zero
	if totalAnimals > 0 {
		vaccinationRate = decimal.NewFromInt(int64(c.Total)).
			Div(decimal.NewFromInt(int64(totalAnimals)))
	}

	cycleCompliance := decimal.Zero
	if a.ReportedCycles > 0 {
		cycleCompliance = decimal.NewFromInt(int64(a.CompletedCycles)).
			Div(decimal.NewFromInt(int64(a.ReportedCycles)))
	}

	seniority := decimal.NewFromInt(int64(a.SeniorityCycles)).Div(decimal.NewFromInt(20))
	if seniority.GreaterThan(decimal.NewFromInt(1)) {
		seniority = decimal.NewFromInt(1)
	}

	tenure := decimal.NewFromFloat(0.7)
	if a.Tenure == TenureOwned {
		tenure = decimal.NewFromInt(1)
	}

	status := decimal.NewFromFloat(0.5)
	if a.HoldingStatus == HoldingActive {
		status = decimal.NewFromInt(1)
	}

	score := vaccinationRate.Mul(decimal.NewFromInt(35)).
		Add(cycleCompliance.Mul(decimal.NewFromInt(30))).
		Add(seniority.Mul(decimal.NewFromInt(20))).
		Add(tenure.Mul(decimal.NewFromInt(10))).
		Add(status.Mul(decimal.NewFromInt(5)))

	return BehaviourScore{
		Score:              int(score.Round(0).IntPart()),
		VaccinationRatePct: int(vaccinationRate.Mul(decimal.NewFromInt(100)).Round(0).IntPart()),
		CycleComplaincePct: int(cycleCompliance.Mul(decimal.NewFromInt(100)).Round(0).IntPart()),
		SeniorityCycles:    a.SeniorityCycles,
		Tenure:             a.Tenure,
		HoldingStatus:      a.HoldingStatus,
	}
}

// IncomeLine is one revenue source over the projection window.
type IncomeLine struct {
	Units  int
	Amount decimal.Decimal
}

// ProjectedIncome is the holding's expected revenue over the credit term.
// Each category is priced with the metric of its own activity, not the
// holding's overall typology.
type ProjectedIncome struct {
	Total decimal.Decimal

	Fattening IncomeLine
	Growing   IncomeLine
	Calves    IncomeLine
	Milk      IncomeLine
	Cull      IncomeLine

	LitresTotal      decimal.Decimal
	LitresPerBreeder decimal.Decimal
}

// ProjectIncome projects revenue over the term. Inventory rotations (25% of
// fattening stock in three months, and so on) are model assumptions, not
// Fedegán data.
func ProjectIncome(c HerdClassification, typology LivestockTypology, department string, p QuotaParams, yields LivestockYields) ProjectedIncome {
	scale := p.rotationScale()

	// Rotation fractions are expressed on a semi-annual base and rescaled.
	rotate := func(head int, fraction string) int {
		frac, _ := decimal.NewFromString(fraction)
		return int(decimal.NewFromInt(int64(head)).Mul(frac).Mul(scale).Floor().IntPart())
	}

	fatteningHead := rotate(c.Fattening, "0.50")
	fattening := IncomeLine{
		Units:  fatteningHead,
		Amount: decimal.NewFromInt(int64(fatteningHead)).Mul(p.WeightFatteningKg).Mul(p.PriceFatteningPerKg),
	}

	growingHead := rotate(c.Growing, "0.25")
	growing := IncomeLine{
		Units:  growingHead,
		Amount: decimal.NewFromInt(int64(growingHead)).Mul(p.WeightGrowingKg).Mul(p.PriceGrowingPerKg),
	}

	calvesHead := rotate(c.Calves, "0.15")
	calves := IncomeLine{
		Units:  calvesHead,
		Amount: decimal.NewFromInt(int64(calvesHead)).Mul(p.WeightCalfKg).Mul(p.PriceCalfPerKg),
	}

	// Milk is a continuous flow: it does not rotate, it scales with the days.
	milkingHead := int(decimal.NewFromInt(int64(c.Breeding)).Mul(p.MilkingShare).Floor().IntPart())
	litresPerBreeder := yields.litresPerCowDay(typology, department).Mul(p.periodDays())
	litresTotal := decimal.NewFromInt(int64(milkingHead)).Mul(litresPerBreeder)
	milk := IncomeLine{
		Units:  milkingHead,
		Amount: litresTotal.Mul(p.PricePerLitre),
	}

	cullHead := rotate(c.Cull, "0.10")
	cull := IncomeLine{
		Units:  cullHead,
		Amount: decimal.NewFromInt(int64(cullHead)).Mul(decimal.NewFromInt(450)).Mul(p.PriceFatteningPerKg),
	}

	return ProjectedIncome{
		Total: fattening.Amount.Add(growing.Amount).Add(calves.Amount).
			Add(milk.Amount).Add(cull.Amount),
		Fattening:        fattening,
		Growing:          growing,
		Calves:           calves,
		Milk:             milk,
		Cull:             cull,
		LitresTotal:      litresTotal,
		LitresPerBreeder: litresPerBreeder,
	}
}

// ProjectedCosts separates all operating costs from the financeable subset.
type ProjectedCosts struct {
	// Total is every operating cost, used for the proforma EBITDA.
	Total decimal.Decimal
	// EligibleBasket is only what the rancher buys in the GANEXPO catalogue and
	// can finance with the quota: vaccines, salt, supplements and vet. Wages and
	// upkeep are excluded because they are not catalogue purchases.
	EligibleBasket decimal.Decimal

	Vaccines    decimal.Decimal
	Salt        decimal.Decimal
	Supplements decimal.Decimal
	Vet         decimal.Decimal
	Wages       decimal.Decimal
	Upkeep      decimal.Decimal

	AppliedRegime string
}

// ProjectCosts computes operating costs over the term.
//
// Labour scales by herd size rather than linearly: without this the model
// charged a full hand to a fifteen-animal holding and handed it a negative
// EBITDA. Family operation is the norm in the target segment.
func ProjectCosts(ugg decimal.Decimal, p QuotaParams) ProjectedCosts {
	yf := p.yearFraction()
	months := decimal.NewFromInt(int64(p.termMonths()))

	vaccines := ugg.Mul(p.VaccineCostPerUGGYear).Mul(yf)
	salt := ugg.Mul(p.SaltCostPerUGGYear).Mul(yf)
	supplements := ugg.Mul(p.SupplementCostPerUGGYear).Mul(yf)
	vet := ugg.Mul(p.VetCostPerUGGYear).Mul(yf)

	regime := p.LabourRegime
	if regime == LabourAuto || regime == "" {
		switch {
		case ugg.LessThan(p.UGGThresholdFamily):
			regime = LabourFamily
		case ugg.LessThan(p.UGGThresholdOccasional):
			regime = "ocasional"
		case ugg.LessThan(p.UGGThresholdPartial):
			regime = "parcial"
		default:
			regime = LabourFormal
		}
	}

	var wages decimal.Decimal
	var appliedRegime string
	switch regime {
	case LabourFamily:
		wages, appliedRegime = decimal.Zero, "Familiar"
	case "ocasional":
		days := decimal.NewFromInt(int64(p.OccasionalDaysPerMonth)).Mul(months)
		wages, appliedRegime = days.Mul(p.DailyWage), "Ocasional"
	case "parcial":
		days := decimal.NewFromInt(13).Mul(months)
		wages, appliedRegime = days.Mul(p.DailyWage), "Parcial"
	default: // formal
		hands := ugg.Div(decimal.NewFromInt(50)).Ceil()
		wages = hands.Mul(decimal.NewFromInt(26)).Mul(months).Mul(p.DailyWage)
		appliedRegime = "Formal"
	}

	hectares := ugg.Mul(p.HectaresPerUGG)
	upkeep := hectares.Mul(p.FenceCostPerHaYear).Mul(yf).
		Add(ugg.Mul(p.InfraCostPerUGGYear).Mul(yf))

	eligible := vaccines.Add(salt).Add(supplements).Add(vet)

	return ProjectedCosts{
		Total:          eligible.Add(wages).Add(upkeep),
		EligibleBasket: eligible,
		Vaccines:       vaccines,
		Salt:           salt,
		Supplements:    supplements,
		Vet:            vet,
		Wages:          wages,
		Upkeep:         upkeep,
		AppliedRegime:  appliedRegime,
	}
}

// Quota ceiling names, in the order the engine evaluates them. On a tie the
// earliest one is reported, matching the reference implementation.
const (
	CeilingLTV       = "LTV sobre flujo"
	CeilingCapacity  = "Capacidad de pago"
	CeilingInputs    = "Necesidad de insumos"
	CeilingPolicyCap = "Tope de política"
)

// LivestockQuota is the assigned quota with the full trace of how it was
// reached. ActiveCeiling answers the risk team's "why is it this number".
type LivestockQuota struct {
	Quota         decimal.Decimal
	ActiveCeiling string

	LTVCeiling      decimal.Decimal
	CapacityCeiling decimal.Decimal
	InputsCeiling   decimal.Decimal
	PolicyCap       decimal.Decimal

	BehaviourMultiplier decimal.Decimal
	PeriodRatePct       decimal.Decimal
	TermMonths          int

	// Unit economics against Fedegán's cost of capital.
	SpreadPoints      decimal.Decimal
	PeriodInterest    decimal.Decimal
	PeriodCapitalCost decimal.Decimal
	GrossMargin       decimal.Decimal

	CapReached bool
}

// periodRate converts an effective annual rate to the term's rate. It uses
// float math for the fractional exponent, the same way the loan amortization
// recalculation does, then returns to decimal.
func periodRate(annualPct decimal.Decimal, months int) decimal.Decimal {
	annual, _ := annualPct.Div(decimal.NewFromInt(100)).Float64()
	r := math.Pow(1+annual, float64(months)/12) - 1
	return decimal.NewFromFloat(r)
}

// CalculateQuota takes the lowest of four ceilings.
//
// The inputs ceiling is what makes the model usable: without it the engine
// authorized fifty million to ranchers who are going to spend six hundred
// thousand pesos on salt and vaccines. A quota nobody can spend in the
// catalogue is not a quota, it is an inflated number.
func CalculateQuota(income ProjectedIncome, score BehaviourScore, costs ProjectedCosts, p QuotaParams) LivestockQuota {
	months := p.termMonths()
	multiplier := score.Multiplier()
	rate := periodRate(p.AnnualRatePct, months)

	ltv := income.Total.Mul(p.LTVPercent).Div(decimal.NewFromInt(100)).Mul(multiplier)

	capacity := income.Total.Mul(decimal.NewFromFloat(0.45)).
		Div(decimal.NewFromInt(1).Add(rate))

	inputs := decimal.NewFromInt(0)
	hasInputsCeiling := p.ApplyInputCeiling
	if hasInputsCeiling {
		headroom := p.InputHeadroom
		if headroom.IsZero() {
			headroom = decimal.NewFromInt(1)
		}
		inputs = costs.EligibleBasket.Mul(headroom)
	}

	type ceiling struct {
		name  string
		value decimal.Decimal
	}
	ceilings := []ceiling{
		{CeilingLTV, ltv},
		{CeilingCapacity, capacity},
	}
	if hasInputsCeiling {
		ceilings = append(ceilings, ceiling{CeilingInputs, inputs})
	}
	ceilings = append(ceilings, ceiling{CeilingPolicyCap, p.MaxQuota})

	best := ceilings[0]
	for _, c := range ceilings[1:] {
		if c.value.LessThan(best.value) {
			best = c
		}
	}

	final := best.value
	if final.IsNegative() {
		final = decimal.Zero
	}

	interest := final.Mul(rate)
	capitalCost := final.Mul(periodRate(p.CapitalCostPct, months))

	return LivestockQuota{
		Quota:               final,
		ActiveCeiling:       best.name,
		LTVCeiling:          ltv,
		CapacityCeiling:     capacity,
		InputsCeiling:       inputs,
		PolicyCap:           p.MaxQuota,
		BehaviourMultiplier: multiplier,
		PeriodRatePct:       rate.Mul(decimal.NewFromInt(100)),
		TermMonths:          months,
		SpreadPoints:        p.AnnualRatePct.Sub(p.CapitalCostPct),
		PeriodInterest:      interest,
		PeriodCapitalCost:   capitalCost,
		GrossMargin:         interest.Sub(capitalCost),
		CapReached:          best.name == CeilingPolicyCap,
	}
}

// LivestockAssessment is the full result of one quota evaluation, kept together
// so the whole calculation is auditable from a single record.
type LivestockAssessment struct {
	Classification HerdClassification
	Typology       LivestockTypology
	UGG            decimal.Decimal
	Score          BehaviourScore
	Income         ProjectedIncome
	Costs          ProjectedCosts
	Quota          LivestockQuota
}

// AssessQuota runs the whole engine for one applicant: classify the herd,
// detect the typology, convert to UGG, project income and costs, score
// behaviour, and take the lowest ceiling.
func AssessQuota(applicant LivestockApplicant, p QuotaParams, yields LivestockYields) LivestockAssessment {
	classification := applicant.Herd.Classify()
	typology := classification.Typology()
	ugg := classification.UGGWith(p.FactorsFor(typology))

	score := applicant.BehaviourScore(classification)
	income := ProjectIncome(classification, typology, applicant.Department, p, yields)
	costs := ProjectCosts(ugg, p)
	quota := CalculateQuota(income, score, costs, p)

	return LivestockAssessment{
		Classification: classification,
		Typology:       typology,
		UGG:            ugg,
		Score:          score,
		Income:         income,
		Costs:          costs,
		Quota:          quota,
	}
}
