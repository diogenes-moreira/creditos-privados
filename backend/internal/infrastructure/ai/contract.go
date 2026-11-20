package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// The answer contract is the same for every provider: the operator's prompt
// decides how to judge an applicant, and this file decides what the judgement
// must look like, so switching providers changes neither.

// outputContract is appended to the operator's prompt. It describes the shape
// of the answer, which is enforced by the structured-output schema below. The
// operator owns the analysis criteria; the adapter owns the output contract, so
// editing the prompt can never break parsing.
const outputContract = `
---
Return your analysis as JSON matching the required schema:
- score: integer 0-100, higher is lower risk.
- grade: one of A, B, C, D, E (A is best).
- decision: approve, manual_review or decline.
- probabilityOfDefault: decimal fraction, e.g. "0.08" for 8%.
- recommendedLimit: total credit principal, as a decimal string with no
  thousands separators or currency symbol. Use "0" when declining.
- recommendedInstallments: the term in months you priced the limit for.
- recommendedInterestRate: annual nominal rate as a percentage,
  e.g. "65" for 65% per year.
- maxMonthlyPayment: the installment you consider affordable, decimal string.
- summary: two or three sentences an analyst would read, naming what decided it.
- factors: the drivers that mattered, each with a short code, a human label,
  the value you read, and whether it helped (positive), hurt (negative), or was
  neutral.

The lender applies its own policy caps after you answer, so recommend what the
applicant can genuinely service rather than the largest number the policy allows.`

// analysisSchema is the structured-output schema the response must satisfy.
// Numeric amounts are strings so decimal precision survives the round trip.
func analysisSchema() map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"score", "grade", "decision", "probabilityOfDefault",
			"recommendedLimit", "recommendedInstallments", "recommendedInterestRate",
			"maxMonthlyPayment", "summary", "factors",
		},
		"properties": map[string]any{
			"score": map[string]any{
				"type":        "integer",
				"description": "Risk score from 0 (worst) to 100 (best).",
			},
			"grade": map[string]any{
				"type": "string",
				"enum": []string{"A", "B", "C", "D", "E"},
			},
			"decision": map[string]any{
				"type": "string",
				"enum": []string{"approve", "manual_review", "decline"},
			},
			"probabilityOfDefault":    str("Expected default probability as a decimal fraction."),
			"recommendedLimit":        str("Recommended credit limit as a decimal amount."),
			"recommendedInterestRate": str("Recommended annual nominal rate as a percentage, e.g. 65 for 65% per year."),
			"maxMonthlyPayment":       str("Affordable monthly installment as a decimal amount."),
			"recommendedInstallments": map[string]any{
				"type":        "integer",
				"description": "Term in months the recommended limit was priced for.",
			},
			"summary": str("Short analyst-facing explanation of the decision."),
			"factors": map[string]any{
				"type":        "array",
				"description": "The drivers behind the score.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"code", "label", "value", "impact", "comment"},
					"properties": map[string]any{
						"code":  str("Short stable identifier, e.g. affordability."),
						"label": str("Human-readable factor name."),
						"value": str("The value observed for this factor."),
						"impact": map[string]any{
							"type": "string",
							"enum": []string{"positive", "negative", "neutral"},
						},
						"comment": str("Why this factor mattered."),
					},
				},
			},
		},
	}
}

// analysisResponse mirrors analysisSchema for decoding.
type analysisResponse struct {
	Score                   int    `json:"score"`
	Grade                   string `json:"grade"`
	Decision                string `json:"decision"`
	ProbabilityOfDefault    string `json:"probabilityOfDefault"`
	RecommendedLimit        string `json:"recommendedLimit"`
	RecommendedInstallments int    `json:"recommendedInstallments"`
	RecommendedInterestRate string `json:"recommendedInterestRate"`
	MaxMonthlyPayment       string `json:"maxMonthlyPayment"`
	Summary                 string `json:"summary"`
	Factors                 []struct {
		Code    string `json:"code"`
		Label   string `json:"label"`
		Value   string `json:"value"`
		Impact  string `json:"impact"`
		Comment string `json:"comment"`
	} `json:"factors"`
}

// decodeAnalysis turns a provider's JSON answer into the port's result. It is
// shared so every provider normalises grades, decisions and impacts the same
// way, whatever casing or spacing the model used.
func decodeAnalysis(payload string) (*port.RiskAnalysisResult, error) {
	var parsed analysisResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return nil, fmt.Errorf("could not decode risk analysis response: %w", err)
	}

	result := &port.RiskAnalysisResult{
		Score:                   parsed.Score,
		Grade:                   strings.ToUpper(strings.TrimSpace(parsed.Grade)),
		Decision:                strings.ToLower(strings.TrimSpace(parsed.Decision)),
		ProbabilityOfDefault:    parsed.ProbabilityOfDefault,
		RecommendedLimit:        parsed.RecommendedLimit,
		RecommendedInstallments: parsed.RecommendedInstallments,
		RecommendedInterestRate: parsed.RecommendedInterestRate,
		MaxMonthlyPayment:       parsed.MaxMonthlyPayment,
		Summary:                 parsed.Summary,
		Raw:                     payload,
	}
	for _, f := range parsed.Factors {
		result.Factors = append(result.Factors, port.RiskAnalysisFactor{
			Code:    f.Code,
			Label:   f.Label,
			Value:   f.Value,
			Impact:  strings.ToLower(strings.TrimSpace(f.Impact)),
			Comment: f.Comment,
		})
	}
	return result, nil
}
