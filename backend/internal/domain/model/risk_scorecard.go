package model

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// probePrincipal is an arbitrary reference principal used to invert an
// amortization schedule. French and German schedules are linear in principal,
// so the installment produced for the probe scales exactly.
var probePrincipal = decimal.NewFromInt(100000)

// Scorecard factor weights. They sum to 1 and define how much each dimension
// contributes to the 0-100 score.
var (
	weightAffordability = decimal.NewFromFloat(0.30)
	weightIndebtedness  = decimal.NewFromFloat(0.20)
	weightEmployment    = decimal.NewFromFloat(0.15)
	weightBureau        = decimal.NewFromFloat(0.20)
	weightBehavior      = decimal.NewFromFloat(0.15)
)

// RiskHistory summarizes a client's repayment behaviour inside the system.
type RiskHistory struct {
	LoansTotal             int
	LoansActive            int
	InstallmentsDue        int
	InstallmentsPaidOnTime int
	InstallmentsLate       int
	MaxDaysLate            int
	CurrentOverdueAmount   decimal.Decimal
	OutstandingBalance     decimal.Decimal
}

// HasTrack reports whether there is enough history to judge behaviour.
func (h RiskHistory) HasTrack() bool {
	return h.InstallmentsDue > 0
}

// OnTimeRate is the share of due installments paid without delay.
func (h RiskHistory) OnTimeRate() decimal.Decimal {
	if h.InstallmentsDue <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(h.InstallmentsPaidOnTime)).
		Div(decimal.NewFromInt(int64(h.InstallmentsDue))).Round(4)
}

// RiskInput is everything the analysis needs about one applicant.
type RiskInput struct {
	Profile   *RiskProfile
	Age       int
	IsBlocked bool
	IVARate   decimal.Decimal
	History   RiskHistory
}

// RiskCapacity is the affordability envelope derived from the profile and the
// model guardrails. It bounds any recommendation, whoever produced it.
type RiskCapacity struct {
	RecognizedIncome   decimal.Decimal
	DisposableIncome   decimal.Decimal
	PaymentToIncomeCap decimal.Decimal
	MaxMonthlyPayment  decimal.Decimal
	MaxInstallments    int
	MaxLimit           decimal.Decimal
	DebtToIncomeRatio  decimal.Decimal
}

// Capacity computes how much monthly payment and principal an applicant can
// carry under this model, before any risk grading is applied.
func (m *RiskModel) Capacity(in RiskInput) RiskCapacity {
	profile := in.Profile
	recognized := profile.RecognizedIncome(m.UnverifiedIncomeHaircut)
	disposable := profile.DisposableIncome(m.UnverifiedIncomeHaircut, m.ExpensePerDependent)

	// Payment capacity is the tighter of the disposable surplus and a fixed
	// share of income, so a low-expense declaration cannot unlock an
	// installment the policy considers unaffordable.
	incomeCap := recognized.Mul(m.MaxPaymentToIncome).Round(2)
	payment := disposable
	if incomeCap.LessThan(payment) {
		payment = incomeCap
	}
	if payment.IsNegative() {
		payment = decimal.Zero
	}

	envelope := RiskCapacity{
		RecognizedIncome:   recognized,
		DisposableIncome:   disposable,
		PaymentToIncomeCap: incomeCap,
		MaxMonthlyPayment:  payment,
		MaxInstallments:    m.MaxInstallments,
		DebtToIncomeRatio:  profile.DebtToIncomeRatio(),
	}
	envelope.MaxLimit = m.LimitFromPayment(payment, m.MaxInstallments, m.BaseInterestRate, in.IVARate)
	return envelope
}

// LimitFromPayment converts an affordable monthly payment into the principal it
// can service, using the French amortization schedule the loans actually use.
// The returned principal is rounded down to the model's rounding step and
// capped at the absolute exposure limit.
func (m *RiskModel) LimitFromPayment(payment decimal.Decimal, installments int, annualRate, ivaRate decimal.Decimal) decimal.Decimal {
	if !payment.IsPositive() || installments < 1 {
		return decimal.Zero
	}
	schedule := CalculateFrenchAmortization(probePrincipal, annualRate, installments, time.Now(), ivaRate)
	peak := decimal.Zero
	for _, inst := range schedule.Installments {
		if inst.Total.GreaterThan(peak) {
			peak = inst.Total
		}
	}
	if !peak.IsPositive() {
		return decimal.Zero
	}
	// payment / (peak / probe) == payment * probe / peak
	principal := payment.Mul(probePrincipal).Div(peak)
	return m.RoundLimit(principal)
}

// Knockouts lists the hard policy rules the applicant fails. A non-empty result
// means the application must be declined regardless of its score.
func (m *RiskModel) Knockouts(in RiskInput) []string {
	var reasons []string
	profile := in.Profile

	if in.IsBlocked {
		reasons = append(reasons, "client is blocked")
	}
	if in.Age < m.MinAge {
		reasons = append(reasons, fmt.Sprintf("age %d is below the minimum of %d", in.Age, m.MinAge))
	}
	if in.Age > m.MaxAge {
		reasons = append(reasons, fmt.Sprintf("age %d exceeds the maximum of %d", in.Age, m.MaxAge))
	}
	if profile.GrossMonthlyIncome().LessThan(m.MinMonthlyIncome) {
		reasons = append(reasons, fmt.Sprintf("declared income %s is below the minimum of %s",
			profile.GrossMonthlyIncome().StringFixed(2), m.MinMonthlyIncome.StringFixed(2)))
	}
	if profile.DebtToIncomeRatio().GreaterThan(m.MaxDTI) {
		reasons = append(reasons, fmt.Sprintf("debt-to-income ratio %s exceeds the maximum of %s",
			profile.DebtToIncomeRatio().StringFixed(4), m.MaxDTI.StringFixed(4)))
	}
	if profile.AdverseRecords > m.MaxAdverseRecords {
		reasons = append(reasons, fmt.Sprintf("%d adverse bureau records exceed the maximum of %d",
			profile.AdverseRecords, m.MaxAdverseRecords))
	}
	if profile.EmploymentType == EmploymentUnemployed {
		reasons = append(reasons, "applicant declares no employment")
	}
	if in.History.CurrentOverdueAmount.IsPositive() {
		reasons = append(reasons, fmt.Sprintf("client has %s currently overdue",
			in.History.CurrentOverdueAmount.StringFixed(2)))
	}
	return reasons
}

// Evaluate runs the deterministic scorecard. It is the baseline analysis: it
// always produces a defensible recommendation, is used directly when no
// prompt-driven analyzer is configured, and is handed to that analyzer as a
// reference when one is.
func (m *RiskModel) Evaluate(in RiskInput) RiskEvaluation {
	capacity := m.Capacity(in)
	eval := RiskEvaluation{
		Analyzer:          AnalyzerScorecard,
		MaxMonthlyPayment: capacity.MaxMonthlyPayment,
	}

	factors := []RiskFactor{
		m.scoreAffordability(capacity),
		m.scoreIndebtedness(in),
		m.scoreEmployment(in),
		m.scoreBureau(in),
		m.scoreBehavior(in),
	}

	total := decimal.Zero
	for _, f := range factors {
		total = total.Add(decimal.NewFromInt(int64(f.Points)).Mul(f.Weight))
	}
	eval.Score = int(total.Round(0).IntPart())
	if eval.Score > 100 {
		eval.Score = 100
	}
	if eval.Score < 0 {
		eval.Score = 0
	}
	eval.Factors = factors

	if knockouts := m.Knockouts(in); len(knockouts) > 0 {
		eval.Decision = RiskDecisionDecline
		eval.Grade = RiskGradeE
		eval.RecommendedLimit = decimal.Zero
		eval.RecommendedInstallments = 0
		eval.RecommendedInterestRate = m.BaseInterestRate
		eval.MaxMonthlyPayment = decimal.Zero
		eval.Summary = "Declined by policy: " + knockouts[0]
		for _, r := range knockouts {
			eval.AddNote("policy knockout: " + r)
		}
		if band := m.BandForGrade(RiskGradeE); band != nil {
			eval.ProbabilityOfDefault = band.ProbabilityOfDefault
		}
		return eval
	}

	band := m.BandFor(eval.Score)
	eval.Grade = RiskGradeE
	eval.Decision = RiskDecisionManualReview
	eval.RecommendedInterestRate = m.BaseInterestRate
	if band != nil {
		eval.Grade = band.Grade
		eval.Decision = band.Decision
		eval.ProbabilityOfDefault = band.ProbabilityOfDefault
		eval.RecommendedInterestRate = m.RateFor(band)
		eval.RecommendedInstallments = m.TermCapFor(band)
	}

	exposure := decimal.NewFromInt(1)
	if band != nil {
		exposure = band.ExposureFactor
	}
	eval.MaxMonthlyPayment = capacity.MaxMonthlyPayment.Mul(exposure).Round(2)
	eval.RecommendedLimit = m.LimitFromPayment(
		eval.MaxMonthlyPayment,
		eval.RecommendedInstallments,
		eval.RecommendedInterestRate,
		in.IVARate,
	)

	if eval.Decision == RiskDecisionDecline || !eval.RecommendedLimit.IsPositive() {
		if eval.Decision != RiskDecisionDecline {
			eval.Decision = RiskDecisionDecline
			eval.AddNote("no affordable installment at the recommended term")
		}
		eval.RecommendedLimit = decimal.Zero
	}

	eval.Summary = fmt.Sprintf(
		"Score %d (grade %s). Affordable installment %s over %d months yields a limit of %s.",
		eval.Score, eval.Grade, eval.MaxMonthlyPayment.StringFixed(2),
		eval.RecommendedInstallments, eval.RecommendedLimit.StringFixed(2),
	)
	return eval
}

// ApplyGuardrails clamps an externally produced evaluation to the model policy.
// It can only tighten: limits shrink, terms shorten, rates rise to the band
// floor and decisions move toward decline. Every clamp is recorded as a note so
// an operator can see what the policy overrode.
func (m *RiskModel) ApplyGuardrails(eval *RiskEvaluation, in RiskInput, capacity RiskCapacity) {
	if eval.Score < 0 {
		eval.Score = 0
	}
	if eval.Score > 100 {
		eval.Score = 100
	}

	band := m.BandFor(eval.Score)
	if band != nil && eval.Grade != band.Grade {
		eval.AddNote(fmt.Sprintf("grade %s replaced with %s to match the band for score %d",
			eval.Grade, band.Grade, eval.Score))
		eval.Grade = band.Grade
	}
	if eval.Grade == "" && band != nil {
		eval.Grade = band.Grade
	}
	if band != nil {
		if !eval.ProbabilityOfDefault.IsPositive() {
			eval.ProbabilityOfDefault = band.ProbabilityOfDefault
		}
		if floor := m.RateFor(band); eval.RecommendedInterestRate.LessThan(floor) {
			if eval.RecommendedInterestRate.IsPositive() {
				eval.AddNote(fmt.Sprintf("interest rate raised from %s to the band floor %s",
					eval.RecommendedInterestRate.StringFixed(4), floor.StringFixed(4)))
			}
			eval.RecommendedInterestRate = floor
		}
		eval.Decision = StricterOf(eval.Decision, band.Decision)
	}
	if !eval.Decision.valid() {
		eval.Decision = RiskDecisionManualReview
		eval.AddNote("missing or invalid decision defaulted to manual review")
	}

	if knockouts := m.Knockouts(in); len(knockouts) > 0 {
		eval.Decision = RiskDecisionDecline
		eval.RecommendedLimit = decimal.Zero
		eval.RecommendedInstallments = 0
		eval.MaxMonthlyPayment = decimal.Zero
		for _, r := range knockouts {
			eval.AddNote("policy knockout: " + r)
		}
		return
	}

	termCap := m.TermCapFor(band)
	if eval.RecommendedInstallments < 1 {
		eval.RecommendedInstallments = termCap
	}
	if eval.RecommendedInstallments > termCap {
		eval.AddNote(fmt.Sprintf("term reduced from %d to the policy cap of %d months",
			eval.RecommendedInstallments, termCap))
		eval.RecommendedInstallments = termCap
	}

	exposure := decimal.NewFromInt(1)
	if band != nil {
		exposure = band.ExposureFactor
	}
	paymentCap := capacity.MaxMonthlyPayment.Mul(exposure).Round(2)
	if !eval.MaxMonthlyPayment.IsPositive() || eval.MaxMonthlyPayment.GreaterThan(paymentCap) {
		if eval.MaxMonthlyPayment.GreaterThan(paymentCap) {
			eval.AddNote(fmt.Sprintf("monthly payment reduced from %s to the affordability cap %s",
				eval.MaxMonthlyPayment.StringFixed(2), paymentCap.StringFixed(2)))
		}
		eval.MaxMonthlyPayment = paymentCap
	}

	limitCap := m.LimitFromPayment(
		eval.MaxMonthlyPayment,
		eval.RecommendedInstallments,
		eval.RecommendedInterestRate,
		in.IVARate,
	)
	if eval.RecommendedLimit.GreaterThan(limitCap) {
		eval.AddNote(fmt.Sprintf("limit reduced from %s to the affordability cap %s",
			eval.RecommendedLimit.StringFixed(2), limitCap.StringFixed(2)))
		eval.RecommendedLimit = limitCap
	}
	eval.RecommendedLimit = m.RoundLimit(eval.RecommendedLimit)

	if !eval.RecommendedLimit.IsPositive() && eval.Decision != RiskDecisionDecline {
		eval.Decision = RiskDecisionDecline
		eval.AddNote("no affordable installment at the recommended term")
	}
}

// --- scorecard dimensions -------------------------------------------------

func (m *RiskModel) scoreAffordability(capacity RiskCapacity) RiskFactor {
	ratio := decimal.Zero
	if capacity.RecognizedIncome.IsPositive() {
		ratio = capacity.MaxMonthlyPayment.Div(capacity.RecognizedIncome)
	}
	// Full marks once the affordable installment reaches the policy cap.
	points := 0
	if m.MaxPaymentToIncome.IsPositive() {
		pct := ratio.Div(m.MaxPaymentToIncome).Mul(decimal.NewFromInt(100))
		points = clampPoints(int(pct.Round(0).IntPart()))
	}
	return RiskFactor{
		Code:   "affordability",
		Label:  "Payment capacity vs income",
		Value:  capacity.MaxMonthlyPayment.StringFixed(2),
		Weight: weightAffordability,
		Points: points,
		Impact: impactFor(points),
		Comment: fmt.Sprintf("Disposable income %s of recognized income %s",
			capacity.DisposableIncome.StringFixed(2), capacity.RecognizedIncome.StringFixed(2)),
	}
}

func (m *RiskModel) scoreIndebtedness(in RiskInput) RiskFactor {
	dti := in.Profile.DebtToIncomeRatio()
	points := 100
	if m.MaxDTI.IsPositive() {
		used := dti.Div(m.MaxDTI).Mul(decimal.NewFromInt(100))
		points = clampPoints(100 - int(used.Round(0).IntPart()))
	}
	return RiskFactor{
		Code:    "indebtedness",
		Label:   "External debt burden",
		Value:   dti.StringFixed(4),
		Weight:  weightIndebtedness,
		Points:  points,
		Impact:  impactFor(points),
		Comment: fmt.Sprintf("Debt-to-income ratio against a policy cap of %s", m.MaxDTI.StringFixed(4)),
	}
}

func (m *RiskModel) scoreEmployment(in RiskInput) RiskFactor {
	profile := in.Profile
	base := 0
	switch profile.EmploymentType {
	case EmploymentEmployee:
		base = 80
	case EmploymentRetired:
		base = 70
	case EmploymentBusinessOwner:
		base = 65
	case EmploymentSelfEmployed:
		base = 55
	case EmploymentInformal:
		base = 30
	case EmploymentUnemployed:
		base = 0
	}
	// Tenure adds up to 20 points, saturating at three years.
	tenure := profile.MonthsEmployed
	if tenure > 36 {
		tenure = 36
	}
	base += tenure * 20 / 36

	if !profile.IncomeVerified {
		base -= 10
	}
	if profile.HasBankAccount {
		base += 5
	}
	switch profile.HousingStatus {
	case HousingOwned:
		base += 5
	case HousingRented:
		base -= 5
	}

	points := clampPoints(base)
	return RiskFactor{
		Code:   "employment",
		Label:  "Employment and income stability",
		Value:  string(profile.EmploymentType),
		Weight: weightEmployment,
		Points: points,
		Impact: impactFor(points),
		Comment: fmt.Sprintf("%d months of tenure, income verified: %t",
			profile.MonthsEmployed, profile.IncomeVerified),
	}
}

func (m *RiskModel) scoreBureau(in RiskInput) RiskFactor {
	profile := in.Profile
	if !profile.HasBureauScore() {
		// No bureau data is neither good nor bad; score it neutrally so an
		// applicant without a file is not punished as if they had a bad one.
		return RiskFactor{
			Code:    "bureau",
			Label:   "External bureau score",
			Value:   "not available",
			Weight:  weightBureau,
			Points:  50,
			Impact:  FactorNeutral,
			Comment: "No external bureau data on file",
		}
	}
	points := clampPoints(*profile.BureauScore * 100 / BureauScoreScale)
	if profile.AdverseRecords > 0 {
		points = clampPoints(points - profile.AdverseRecords*20)
	}
	return RiskFactor{
		Code:    "bureau",
		Label:   "External bureau score",
		Value:   fmt.Sprintf("%d/%d", *profile.BureauScore, BureauScoreScale),
		Weight:  weightBureau,
		Points:  points,
		Impact:  impactFor(points),
		Comment: fmt.Sprintf("%d adverse records", profile.AdverseRecords),
	}
}

func (m *RiskModel) scoreBehavior(in RiskInput) RiskFactor {
	h := in.History
	if !h.HasTrack() {
		return RiskFactor{
			Code:    "behavior",
			Label:   "Internal repayment history",
			Value:   "no history",
			Weight:  weightBehavior,
			Points:  50,
			Impact:  FactorNeutral,
			Comment: "First-time applicant with no repayment track record",
		}
	}
	points := int(h.OnTimeRate().Mul(decimal.NewFromInt(100)).Round(0).IntPart())
	switch {
	case h.MaxDaysLate > 90:
		points -= 40
	case h.MaxDaysLate > 60:
		points -= 25
	case h.MaxDaysLate > 30:
		points -= 15
	}
	points = clampPoints(points)
	return RiskFactor{
		Code:   "behavior",
		Label:  "Internal repayment history",
		Value:  h.OnTimeRate().Mul(decimal.NewFromInt(100)).StringFixed(1) + "% on time",
		Weight: weightBehavior,
		Points: points,
		Impact: impactFor(points),
		Comment: fmt.Sprintf("%d of %d installments paid on time, worst delay %d days",
			h.InstallmentsPaidOnTime, h.InstallmentsDue, h.MaxDaysLate),
	}
}

func clampPoints(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func impactFor(points int) FactorImpact {
	switch {
	case points >= 60:
		return FactorPositive
	case points <= 40:
		return FactorNegative
	default:
		return FactorNeutral
	}
}
