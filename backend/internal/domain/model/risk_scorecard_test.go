package model_test

import (
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testIVARate = decimal.NewFromInt(21)

// strongApplicant is a well-qualified applicant against the AR defaults.
func strongApplicant(t *testing.T) model.RiskInput {
	t.Helper()
	score := 850
	in := model.RiskProfileInput{
		MonthlyIncome:        decimal.NewFromInt(1500000),
		AdditionalIncome:     decimal.Zero,
		IncomeVerified:       true,
		EmploymentType:       model.EmploymentEmployee,
		EmployerName:         "Acme",
		MonthsEmployed:       60,
		MonthlyExpenses:      decimal.NewFromInt(300000),
		ExistingDebtPayments: decimal.NewFromInt(50000),
		Dependents:           0,
		HousingStatus:        model.HousingOwned,
		HasBankAccount:       true,
		BureauScore:          &score,
	}
	profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceAdmin, nil)
	require.NoError(t, err)

	return model.RiskInput{
		Profile: profile,
		Age:     40,
		IVARate: testIVARate,
		History: model.RiskHistory{
			LoansTotal:             3,
			InstallmentsDue:        24,
			InstallmentsPaidOnTime: 24,
			CurrentOverdueAmount:   decimal.Zero,
			OutstandingBalance:     decimal.Zero,
		},
	}
}

// weakApplicant is a poorly-qualified but not knocked-out applicant.
func weakApplicant(t *testing.T) model.RiskInput {
	t.Helper()
	score := 380
	in := model.RiskProfileInput{
		MonthlyIncome:        decimal.NewFromInt(400000),
		IncomeVerified:       false,
		EmploymentType:       model.EmploymentInformal,
		MonthsEmployed:       4,
		MonthlyExpenses:      decimal.NewFromInt(250000),
		ExistingDebtPayments: decimal.NewFromInt(60000),
		Dependents:           3,
		HousingStatus:        model.HousingRented,
		HasBankAccount:       false,
		BureauScore:          &score,
	}
	profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceAdmin, nil)
	require.NoError(t, err)

	return model.RiskInput{
		Profile: profile,
		Age:     30,
		IVARate: testIVARate,
		History: model.RiskHistory{
			LoansTotal:             2,
			InstallmentsDue:        12,
			InstallmentsPaidOnTime: 6,
			InstallmentsLate:       6,
			MaxDaysLate:            75,
			CurrentOverdueAmount:   decimal.Zero,
		},
	}
}

func TestRiskModelCapacity(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)

	capacity := m.Capacity(in)

	assert.True(t, capacity.RecognizedIncome.Equal(decimal.NewFromInt(1500000)))
	// Disposable: 1500000 - 350000 - 0 dependents = 1150000.
	assert.True(t, capacity.DisposableIncome.Equal(decimal.NewFromInt(1150000)),
		"got %s", capacity.DisposableIncome)
	// Income cap: 30% of 1500000 = 450000, which is the binding constraint.
	assert.True(t, capacity.MaxMonthlyPayment.Equal(decimal.NewFromInt(450000)),
		"the payment-to-income cap must bind when disposable income is larger, got %s",
		capacity.MaxMonthlyPayment)
	assert.True(t, capacity.MaxLimit.IsPositive())
}

func TestRiskModelCapacityBoundByDisposableIncome(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	// Push expenses up so the surplus, not the income share, is the constraint.
	in.Profile.MonthlyExpenses = decimal.NewFromInt(1300000)

	capacity := m.Capacity(in)

	// Disposable: 1500000 - 1350000 = 150000, below the 450000 income cap.
	assert.True(t, capacity.MaxMonthlyPayment.Equal(decimal.NewFromInt(150000)),
		"got %s", capacity.MaxMonthlyPayment)
}

func TestRiskModelCapacityWithNoSurplus(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	in.Profile.MonthlyExpenses = decimal.NewFromInt(2000000)

	capacity := m.Capacity(in)

	assert.True(t, capacity.MaxMonthlyPayment.IsZero())
	assert.True(t, capacity.MaxLimit.IsZero(),
		"no payment capacity means no credit availability")
}

// The limit is derived by inverting the amortization schedule, so feeding it
// back through that schedule must not produce an installment the applicant
// cannot afford. This is the core guarantee of the availability calculation.
func TestLimitFromPaymentIsAffordableWhenAmortized(t *testing.T) {
	m := newTestModel(t)
	rate := decimal.NewFromInt(60)

	for _, payment := range []int64{50000, 120000, 450000, 1000000} {
		for _, term := range []int{3, 6, 12, 24} {
			p := decimal.NewFromInt(payment)
			limit := m.LimitFromPayment(p, term, rate, testIVARate)
			require.True(t, limit.IsPositive(),
				"payment %s over %d months should support some principal", p, term)

			schedule := model.CalculateFrenchAmortization(limit, rate, term, time.Now(), testIVARate)
			peak := decimal.Zero
			for _, inst := range schedule.Installments {
				if inst.Total.GreaterThan(peak) {
					peak = inst.Total
				}
			}
			assert.False(t, peak.GreaterThan(p),
				"payment %s over %d months yielded limit %s whose largest installment is %s",
				p, term, limit, peak)
		}
	}
}

func TestLimitFromPaymentEdgeCases(t *testing.T) {
	m := newTestModel(t)
	rate := decimal.NewFromInt(60)

	assert.True(t, m.LimitFromPayment(decimal.Zero, 12, rate, testIVARate).IsZero())
	assert.True(t, m.LimitFromPayment(decimal.NewFromInt(-100), 12, rate, testIVARate).IsZero())
	assert.True(t, m.LimitFromPayment(decimal.NewFromInt(100000), 0, rate, testIVARate).IsZero())
}

func TestLimitFromPaymentGrowsWithTerm(t *testing.T) {
	m := newTestModel(t)
	rate := decimal.NewFromInt(60)
	payment := decimal.NewFromInt(200000)

	short := m.LimitFromPayment(payment, 6, rate, testIVARate)
	long := m.LimitFromPayment(payment, 24, rate, testIVARate)

	assert.True(t, long.GreaterThan(short),
		"the same installment over a longer term must support more principal: %s vs %s", short, long)
}

func TestRiskModelKnockouts(t *testing.T) {
	m := newTestModel(t)

	tests := []struct {
		name   string
		mutate func(*model.RiskInput)
		want   string
	}{
		{
			name:   "blocked client",
			mutate: func(in *model.RiskInput) { in.IsBlocked = true },
			want:   "client is blocked",
		},
		{
			name:   "under the minimum age",
			mutate: func(in *model.RiskInput) { in.Age = 17 },
			want:   "below the minimum",
		},
		{
			name:   "over the maximum age",
			mutate: func(in *model.RiskInput) { in.Age = 90 },
			want:   "exceeds the maximum",
		},
		{
			name:   "income below the floor",
			mutate: func(in *model.RiskInput) { in.Profile.MonthlyIncome = decimal.NewFromInt(1000) },
			want:   "below the minimum",
		},
		{
			name: "debt-to-income above the cap",
			mutate: func(in *model.RiskInput) {
				in.Profile.ExistingDebtPayments = in.Profile.GrossMonthlyIncome().Mul(decimal.NewFromFloat(0.6))
			},
			want: "debt-to-income ratio",
		},
		{
			name:   "adverse bureau records",
			mutate: func(in *model.RiskInput) { in.Profile.AdverseRecords = 2 },
			want:   "adverse bureau records",
		},
		{
			name:   "declared unemployment",
			mutate: func(in *model.RiskInput) { in.Profile.EmploymentType = model.EmploymentUnemployed },
			want:   "no employment",
		},
		{
			name: "currently in arrears",
			mutate: func(in *model.RiskInput) {
				in.History.CurrentOverdueAmount = decimal.NewFromInt(5000)
			},
			want: "currently overdue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strongApplicant(t)
			require.Empty(t, m.Knockouts(in), "the baseline applicant must pass policy")

			tt.mutate(&in)
			reasons := m.Knockouts(in)

			require.NotEmpty(t, reasons)
			joined := ""
			for _, r := range reasons {
				joined += r + "|"
			}
			assert.Contains(t, joined, tt.want)
		})
	}
}

func TestRiskModelEvaluateStrongApplicant(t *testing.T) {
	m := newTestModel(t)

	eval := m.Evaluate(strongApplicant(t))

	assert.Equal(t, model.AnalyzerScorecard, eval.Analyzer)
	assert.GreaterOrEqual(t, eval.Score, 80, "a strong applicant should reach grade A")
	assert.Equal(t, model.RiskGradeA, eval.Grade)
	assert.Equal(t, model.RiskDecisionApprove, eval.Decision)
	assert.True(t, eval.RecommendedLimit.IsPositive())
	assert.Equal(t, 24, eval.RecommendedInstallments)
	assert.True(t, eval.RecommendedInterestRate.Equal(m.BaseInterestRate))
	assert.Len(t, eval.Factors, 5, "every scorecard dimension must be explained")
	assert.NotEmpty(t, eval.Summary)
}

func TestRiskModelEvaluateWeakApplicant(t *testing.T) {
	m := newTestModel(t)

	eval := m.Evaluate(weakApplicant(t))

	assert.Less(t, eval.Score, 65, "a weak applicant must not reach grade B")
	assert.NotEqual(t, model.RiskDecisionApprove, eval.Decision)
	assert.True(t, eval.RecommendedInterestRate.GreaterThan(m.BaseInterestRate),
		"a worse grade must be priced above the base rate")
}

func TestRiskModelEvaluateScoreIsBounded(t *testing.T) {
	m := newTestModel(t)

	for _, in := range []model.RiskInput{strongApplicant(t), weakApplicant(t)} {
		eval := m.Evaluate(in)
		assert.GreaterOrEqual(t, eval.Score, 0)
		assert.LessOrEqual(t, eval.Score, 100)
	}
}

func TestRiskModelEvaluateKnockoutDeclines(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	in.IsBlocked = true

	eval := m.Evaluate(in)

	assert.Equal(t, model.RiskDecisionDecline, eval.Decision)
	assert.Equal(t, model.RiskGradeE, eval.Grade)
	assert.True(t, eval.RecommendedLimit.IsZero(), "a declined applicant gets no limit")
	assert.True(t, eval.MaxMonthlyPayment.IsZero())
	assert.Equal(t, 0, eval.RecommendedInstallments)
	require.NotEmpty(t, eval.PolicyNotes)
	assert.Contains(t, eval.PolicyNotes[0], "policy knockout")
}

func TestRiskModelEvaluateFactorsCoverEveryDimension(t *testing.T) {
	m := newTestModel(t)

	eval := m.Evaluate(strongApplicant(t))

	codes := map[string]bool{}
	for _, f := range eval.Factors {
		codes[f.Code] = true
		assert.NotEmpty(t, f.Label, "factor %s needs a human label", f.Code)
		assert.GreaterOrEqual(t, f.Points, 0)
		assert.LessOrEqual(t, f.Points, 100)
	}
	for _, want := range []string{"affordability", "indebtedness", "employment", "bureau", "behavior"} {
		assert.True(t, codes[want], "missing factor %s", want)
	}
}

func TestRiskModelEvaluateScoresMissingDataNeutrally(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	in.Profile.BureauScore = nil
	in.History = model.RiskHistory{}

	eval := m.Evaluate(in)

	for _, f := range eval.Factors {
		if f.Code == "bureau" || f.Code == "behavior" {
			assert.Equal(t, 50, f.Points,
				"%s must be neutral when no data exists, not penalised", f.Code)
			assert.Equal(t, model.FactorNeutral, f.Impact)
		}
	}
}

func TestApplyGuardrailsOnlyTightens(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	capacity := m.Capacity(in)

	// An analyzer proposing far more than policy allows, at a longer term and a
	// cheaper rate than its grade permits.
	eval := model.RiskEvaluation{
		Analyzer:                model.AnalyzerLLM,
		Score:                   40, // grade D: manual review, 6-month cap
		Grade:                   model.RiskGradeA,
		Decision:                model.RiskDecisionApprove,
		MaxMonthlyPayment:       decimal.NewFromInt(99999999),
		RecommendedLimit:        decimal.NewFromInt(99999999),
		RecommendedInstallments: 60,
		RecommendedInterestRate: decimal.NewFromInt(1),
	}

	m.ApplyGuardrails(&eval, in, capacity)

	assert.Equal(t, model.RiskGradeD, eval.Grade, "the grade is corrected to match the score")
	assert.Equal(t, model.RiskDecisionManualReview, eval.Decision,
		"guardrails may only tighten the decision")
	assert.Equal(t, 6, eval.RecommendedInstallments, "the band term cap applies")
	assert.True(t, eval.RecommendedInterestRate.GreaterThanOrEqual(m.BaseInterestRate),
		"the rate is raised to the band floor")
	assert.True(t, eval.RecommendedLimit.LessThan(decimal.NewFromInt(99999999)))
	assert.False(t, eval.RecommendedLimit.GreaterThan(m.AbsoluteMaxLimit))
	assert.NotEmpty(t, eval.PolicyNotes, "every clamp must be explained")
}

func TestApplyGuardrailsKeepsConservativeRecommendation(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	capacity := m.Capacity(in)

	// A modest recommendation well inside policy must survive untouched.
	eval := model.RiskEvaluation{
		Analyzer:                model.AnalyzerLLM,
		Score:                   85,
		Grade:                   model.RiskGradeA,
		Decision:                model.RiskDecisionApprove,
		MaxMonthlyPayment:       decimal.NewFromInt(100000),
		RecommendedLimit:        decimal.NewFromInt(500000),
		RecommendedInstallments: 12,
		RecommendedInterestRate: decimal.NewFromInt(70),
	}

	m.ApplyGuardrails(&eval, in, capacity)

	assert.Equal(t, model.RiskDecisionApprove, eval.Decision)
	assert.Equal(t, 12, eval.RecommendedInstallments)
	assert.True(t, eval.RecommendedLimit.Equal(decimal.NewFromInt(500000)),
		"a limit inside policy must not be reduced, got %s", eval.RecommendedLimit)
	assert.True(t, eval.RecommendedInterestRate.Equal(decimal.NewFromInt(70)),
		"a rate above the band floor must not be lowered")
}

func TestApplyGuardrailsDeclinesOnKnockout(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	in.Profile.AdverseRecords = 3
	capacity := m.Capacity(in)

	eval := model.RiskEvaluation{
		Score:                   95,
		Grade:                   model.RiskGradeA,
		Decision:                model.RiskDecisionApprove,
		RecommendedLimit:        decimal.NewFromInt(1000000),
		RecommendedInstallments: 24,
	}

	m.ApplyGuardrails(&eval, in, capacity)

	assert.Equal(t, model.RiskDecisionDecline, eval.Decision,
		"a policy knockout overrides any score the analyzer produced")
	assert.True(t, eval.RecommendedLimit.IsZero())
	assert.Equal(t, 0, eval.RecommendedInstallments)
}

func TestApplyGuardrailsNormalizesInvalidValues(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	capacity := m.Capacity(in)

	// An unrecognized decision must not be trusted as an approval. With a
	// usable limit it degrades to manual review rather than approving.
	eval := model.RiskEvaluation{
		Score:                   150,
		Decision:                "nonsense",
		RecommendedLimit:        decimal.NewFromInt(200000),
		RecommendedInstallments: 12,
		MaxMonthlyPayment:       decimal.NewFromInt(80000),
	}
	m.ApplyGuardrails(&eval, in, capacity)

	assert.Equal(t, 100, eval.Score, "an out-of-range score is clamped")
	assert.Equal(t, model.RiskGradeA, eval.Grade, "a missing grade is derived from the score")
	assert.Equal(t, model.RiskDecisionManualReview, eval.Decision,
		"an unrecognized decision degrades to manual review, never to approval")

	// With no limit proposed there is nothing to approve.
	empty := model.RiskEvaluation{Score: 90, Decision: "nonsense"}
	m.ApplyGuardrails(&empty, in, capacity)
	assert.Equal(t, model.RiskDecisionDecline, empty.Decision,
		"an analyzer that recommends no credit results in a decline")

	low := model.RiskEvaluation{Score: -20, Decision: model.RiskDecisionApprove}
	m.ApplyGuardrails(&low, in, capacity)
	assert.Equal(t, 0, low.Score)
	assert.Equal(t, model.RiskGradeE, low.Grade)
	assert.Equal(t, model.RiskDecisionDecline, low.Decision,
		"grade E declines regardless of what the analyzer asked for")
}

func TestApplyGuardrailsDeclinesWhenNothingIsAffordable(t *testing.T) {
	m := newTestModel(t)
	in := strongApplicant(t)
	in.Profile.MonthlyExpenses = decimal.NewFromInt(1490000) // leaves almost no surplus
	capacity := m.Capacity(in)

	eval := model.RiskEvaluation{
		Score:                   90,
		Grade:                   model.RiskGradeA,
		Decision:                model.RiskDecisionApprove,
		RecommendedLimit:        decimal.NewFromInt(1000000),
		RecommendedInstallments: 24,
	}
	m.ApplyGuardrails(&eval, in, capacity)

	assert.Equal(t, model.RiskDecisionDecline, eval.Decision)
	assert.True(t, eval.RecommendedLimit.IsZero())
}

func TestRiskHistoryOnTimeRate(t *testing.T) {
	assert.True(t, model.RiskHistory{}.OnTimeRate().IsZero(),
		"no history yields no rate rather than dividing by zero")
	assert.False(t, model.RiskHistory{}.HasTrack())

	h := model.RiskHistory{InstallmentsDue: 10, InstallmentsPaidOnTime: 7}
	assert.True(t, h.HasTrack())
	assert.True(t, h.OnTimeRate().Equal(decimal.NewFromFloat(0.7)))
}

func TestRiskEvaluationAddNoteDeduplicates(t *testing.T) {
	eval := model.RiskEvaluation{}

	eval.AddNote("same")
	eval.AddNote("same")
	eval.AddNote("other")

	assert.Equal(t, []string{"same", "other"}, eval.PolicyNotes)
}
