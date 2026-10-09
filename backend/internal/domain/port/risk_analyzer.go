package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
)

// RiskAnalysisRequest is one applicant handed to an analyzer for judgement.
// Prompt is already rendered by the domain model, so the adapter never has to
// know about placeholders or policy.
type RiskAnalysisRequest struct {
	Prompt    string
	Applicant string
	Baseline  model.RiskEvaluation
	Capacity  model.RiskCapacity
}

// RiskAnalysisFactor is one explanatory driver returned by an analyzer.
type RiskAnalysisFactor struct {
	Code    string
	Label   string
	Value   string
	Impact  string
	Comment string
}

// RiskAnalysisResult is an analyzer's raw judgement. Numeric fields arrive as
// strings so no precision is lost before they become decimals, and nothing here
// is trusted: the domain model clamps all of it against policy guardrails.
type RiskAnalysisResult struct {
	Score                   int
	Grade                   string
	ProbabilityOfDefault    string
	Decision                string
	RecommendedLimit        string
	RecommendedInstallments int
	RecommendedInterestRate string
	MaxMonthlyPayment       string
	Summary                 string
	Factors                 []RiskAnalysisFactor
	Raw                     string
}

// ToEvaluation converts an analyzer result into a domain evaluation, parsing
// the numeric fields leniently: an unparseable value becomes zero and is then
// filled in by the guardrail pass.
func (r *RiskAnalysisResult) ToEvaluation(analyzer string) model.RiskEvaluation {
	eval := model.RiskEvaluation{
		Analyzer:                analyzer,
		Score:                   r.Score,
		Grade:                   model.RiskGrade(r.Grade),
		Decision:                model.RiskDecision(r.Decision),
		ProbabilityOfDefault:    parseDecimal(r.ProbabilityOfDefault),
		RecommendedLimit:        parseDecimal(r.RecommendedLimit),
		RecommendedInstallments: r.RecommendedInstallments,
		RecommendedInterestRate: parseDecimal(r.RecommendedInterestRate),
		MaxMonthlyPayment:       parseDecimal(r.MaxMonthlyPayment),
		Summary:                 r.Summary,
		RawResponse:             r.Raw,
	}
	for _, f := range r.Factors {
		impact := model.FactorImpact(f.Impact)
		switch impact {
		case model.FactorPositive, model.FactorNegative, model.FactorNeutral:
		default:
			impact = model.FactorNeutral
		}
		eval.Factors = append(eval.Factors, model.RiskFactor{
			Code:    f.Code,
			Label:   f.Label,
			Value:   f.Value,
			Impact:  impact,
			Comment: f.Comment,
		})
	}
	return eval
}

func parseDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// RiskAnalyzer runs the operator-managed analysis prompt over an applicant.
// Implementations are infrastructure adapters; the deterministic scorecard in
// the domain is used when none is configured.
type RiskAnalyzer interface {
	Name() string
	Analyze(ctx context.Context, req RiskAnalysisRequest) (*RiskAnalysisResult, error)
}
