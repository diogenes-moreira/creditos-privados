package model

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DefaultAnalysisPrompt is the starting analysis prompt shipped with a new
// deployment. Operators edit it from the admin console; the placeholders are
// filled in by the risk service at evaluation time.
const DefaultAnalysisPrompt = `You are a credit risk analyst for a microcredit lender operating in {{country}}.
Amounts are expressed in {{currency}}. Today is {{today}}.

Assess the applicant below and recommend how much credit they can service.

## Applicant
{{client}}

## Declared risk data
{{profile}}

## Repayment history with us
{{history}}

## Lending policy
{{policy}}

## Reference scorecard
A deterministic scorecard already produced this baseline. Treat it as a starting
point, not an answer: depart from it when the data supports a different reading,
and say why in your summary.
{{baseline}}

## How to analyse
1. Weigh payment capacity first. Income that is not verified deserves less
   weight than income that is.
2. Judge stability, not just level: employment type, tenure and household
   dependents change how reliable an income is.
3. Read the repayment history as behaviour. A short but clean track record is
   worth more than none; recent delinquency outweighs an old one.
4. Consider external bureau data where present. Its absence is not a negative.
5. Recommend a limit the applicant can service comfortably at the term you
   propose, not the maximum the policy would tolerate.
6. Be explicit about what drove the decision. Name the factors that mattered and
   whether each helped or hurt.

Score the applicant from 0 (worst) to 100 (best) and recommend approve,
manual_review or decline. Recommend a limit of 0 when declining.`

// DefaultRiskGradeBands are the shipped score bands: A and B auto-approve, C
// goes to a human, D and E are declined.
func DefaultRiskGradeBands() []RiskGradeBand {
	return []RiskGradeBand{
		{
			Grade:                RiskGradeA,
			MinScore:             80,
			InterestSpread:       decimal.Zero,
			ExposureFactor:       decimal.NewFromFloat(1.00),
			MaxInstallments:      24,
			Decision:             RiskDecisionApprove,
			ProbabilityOfDefault: decimal.NewFromFloat(0.02),
		},
		{
			Grade:                RiskGradeB,
			MinScore:             65,
			InterestSpread:       decimal.NewFromInt(5),
			ExposureFactor:       decimal.NewFromFloat(0.85),
			MaxInstallments:      18,
			Decision:             RiskDecisionApprove,
			ProbabilityOfDefault: decimal.NewFromFloat(0.05),
		},
		{
			Grade:                RiskGradeC,
			MinScore:             50,
			InterestSpread:       decimal.NewFromInt(12),
			ExposureFactor:       decimal.NewFromFloat(0.65),
			MaxInstallments:      12,
			Decision:             RiskDecisionManualReview,
			ProbabilityOfDefault: decimal.NewFromFloat(0.12),
		},
		{
			Grade:                RiskGradeD,
			MinScore:             35,
			InterestSpread:       decimal.NewFromInt(20),
			ExposureFactor:       decimal.NewFromFloat(0.40),
			MaxInstallments:      6,
			Decision:             RiskDecisionManualReview,
			ProbabilityOfDefault: decimal.NewFromFloat(0.25),
		},
		{
			Grade:                RiskGradeE,
			MinScore:             0,
			InterestSpread:       decimal.NewFromInt(30),
			ExposureFactor:       decimal.Zero,
			MaxInstallments:      3,
			Decision:             RiskDecisionDecline,
			ProbabilityOfDefault: decimal.NewFromFloat(0.45),
		},
	}
}

// DefaultRiskModelParams returns guardrails scaled to the deployment country.
// Amounts are in local currency, so the income floor and exposure ceiling
// differ between markets.
func DefaultRiskModelParams(country string) RiskModelParams {
	params := RiskModelParams{
		MinAge:                  18,
		MaxAge:                  75,
		MaxAdverseRecords:       0,
		MaxDTI:                  decimal.NewFromFloat(0.45),
		MaxPaymentToIncome:      decimal.NewFromFloat(0.30),
		UnverifiedIncomeHaircut: decimal.NewFromFloat(0.20),
		MaxInstallments:         24,
		BaseInterestRate:        decimal.NewFromInt(60),
	}
	switch country {
	case "CO":
		params.MinMonthlyIncome = decimal.NewFromInt(1300000)
		params.ExpensePerDependent = decimal.NewFromInt(200000)
		params.AbsoluteMaxLimit = decimal.NewFromInt(20000000)
		params.LimitRoundingStep = decimal.NewFromInt(50000)
	default: // AR
		params.MinMonthlyIncome = decimal.NewFromInt(300000)
		params.ExpensePerDependent = decimal.NewFromInt(50000)
		params.AbsoluteMaxLimit = decimal.NewFromInt(5000000)
		params.LimitRoundingStep = decimal.NewFromInt(10000)
	}
	return params
}

// DefaultRiskModel builds the active model a fresh deployment starts with.
func DefaultRiskModel(country string, createdBy *uuid.UUID) (*RiskModel, error) {
	m, err := NewRiskModel(
		"Default credit risk model",
		DefaultAnalysisPrompt,
		"Shipped analysis model. Edit the prompt and guardrails to match your credit policy.",
		DefaultRiskModelParams(country),
		DefaultRiskGradeBands(),
		createdBy,
	)
	if err != nil {
		return nil, err
	}
	m.Activate()
	return m, nil
}
