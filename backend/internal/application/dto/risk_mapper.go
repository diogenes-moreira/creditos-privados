package dto

import (
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
)

const timeLayout = "2006-01-02T15:04:05Z"

// --- requests -> domain ---------------------------------------------------

// ToRiskProfileInput converts a submitted payload into validated domain input.
// Optional amounts default to zero; a malformed amount is an error rather than
// a silent zero, so a typo cannot quietly understate an applicant's expenses.
func ToRiskProfileInput(req SubmitRiskProfileRequest) (model.RiskProfileInput, error) {
	var in model.RiskProfileInput

	monthlyIncome, err := requiredDecimal("monthlyIncome", req.MonthlyIncome)
	if err != nil {
		return in, err
	}
	additionalIncome, err := optionalDecimal("additionalIncome", req.AdditionalIncome)
	if err != nil {
		return in, err
	}
	monthlyExpenses, err := optionalDecimal("monthlyExpenses", req.MonthlyExpenses)
	if err != nil {
		return in, err
	}
	existingDebt, err := optionalDecimal("existingDebtPayments", req.ExistingDebtPayments)
	if err != nil {
		return in, err
	}

	return model.RiskProfileInput{
		MonthlyIncome:        monthlyIncome,
		AdditionalIncome:     additionalIncome,
		IncomeVerified:       req.IncomeVerified,
		EmploymentType:       model.EmploymentType(req.EmploymentType),
		EmployerName:         req.EmployerName,
		MonthsEmployed:       req.MonthsEmployed,
		MonthlyExpenses:      monthlyExpenses,
		ExistingDebtPayments: existingDebt,
		Dependents:           req.Dependents,
		HousingStatus:        model.HousingStatus(req.HousingStatus),
		HasBankAccount:       req.HasBankAccount,
		BureauScore:          req.BureauScore,
		AdverseRecords:       req.AdverseRecords,
		Notes:                req.Notes,
	}, nil
}

// ToRiskModelParams converts guardrail input into domain parameters.
func ToRiskModelParams(req RiskModelParamsRequest) (model.RiskModelParams, error) {
	var params model.RiskModelParams

	fields := []struct {
		name     string
		raw      string
		required bool
		target   *decimal.Decimal
	}{
		{"minMonthlyIncome", req.MinMonthlyIncome, false, &params.MinMonthlyIncome},
		{"maxDti", req.MaxDTI, true, &params.MaxDTI},
		{"maxPaymentToIncome", req.MaxPaymentToIncome, true, &params.MaxPaymentToIncome},
		{"unverifiedIncomeHaircut", req.UnverifiedIncomeHaircut, false, &params.UnverifiedIncomeHaircut},
		{"expensePerDependent", req.ExpensePerDependent, false, &params.ExpensePerDependent},
		{"absoluteMaxLimit", req.AbsoluteMaxLimit, true, &params.AbsoluteMaxLimit},
		{"limitRoundingStep", req.LimitRoundingStep, false, &params.LimitRoundingStep},
		{"baseInterestRate", req.BaseInterestRate, true, &params.BaseInterestRate},
	}
	for _, f := range fields {
		var (
			v   decimal.Decimal
			err error
		)
		if f.required {
			v, err = requiredDecimal(f.name, f.raw)
		} else {
			v, err = optionalDecimal(f.name, f.raw)
		}
		if err != nil {
			return params, err
		}
		*f.target = v
	}

	params.MinAge = req.MinAge
	params.MaxAge = req.MaxAge
	params.MaxAdverseRecords = req.MaxAdverseRecords
	params.MaxInstallments = req.MaxInstallments
	return params, nil
}

// ToRiskGradeBands converts band input into domain bands.
func ToRiskGradeBands(reqs []RiskGradeBandRequest) ([]model.RiskGradeBand, error) {
	bands := make([]model.RiskGradeBand, 0, len(reqs))
	for _, r := range reqs {
		spread, err := optionalDecimal("interestSpread", r.InterestSpread)
		if err != nil {
			return nil, err
		}
		exposure, err := optionalDecimal("exposureFactor", r.ExposureFactor)
		if err != nil {
			return nil, err
		}
		pd, err := optionalDecimal("probabilityOfDefault", r.ProbabilityOfDefault)
		if err != nil {
			return nil, err
		}
		bands = append(bands, model.RiskGradeBand{
			Grade:                model.RiskGrade(r.Grade),
			MinScore:             r.MinScore,
			InterestSpread:       spread,
			ExposureFactor:       exposure,
			MaxInstallments:      r.MaxInstallments,
			Decision:             model.RiskDecision(r.Decision),
			ProbabilityOfDefault: pd,
		})
	}
	return bands, nil
}

func requiredDecimal(name, raw string) (decimal.Decimal, error) {
	if raw == "" {
		return decimal.Zero, fmt.Errorf("%s is required", name)
	}
	v, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid %s: %q is not a number", name, raw)
	}
	return v, nil
}

func optionalDecimal(name, raw string) (decimal.Decimal, error) {
	if raw == "" {
		return decimal.Zero, nil
	}
	v, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid %s: %q is not a number", name, raw)
	}
	return v, nil
}

// --- domain -> responses --------------------------------------------------

func ToRiskProfileResponse(p *model.RiskProfile, clientName string) RiskProfileResponse {
	return RiskProfileResponse{
		ID:                   p.ID.String(),
		ClientID:             p.ClientID.String(),
		ClientName:           clientName,
		MonthlyIncome:        p.MonthlyIncome.StringFixed(2),
		AdditionalIncome:     p.AdditionalIncome.StringFixed(2),
		GrossMonthlyIncome:   p.GrossMonthlyIncome().StringFixed(2),
		IncomeVerified:       p.IncomeVerified,
		EmploymentType:       string(p.EmploymentType),
		EmployerName:         p.EmployerName,
		MonthsEmployed:       p.MonthsEmployed,
		MonthlyExpenses:      p.MonthlyExpenses.StringFixed(2),
		ExistingDebtPayments: p.ExistingDebtPayments.StringFixed(2),
		Dependents:           p.Dependents,
		HousingStatus:        string(p.HousingStatus),
		HasBankAccount:       p.HasBankAccount,
		BureauScore:          p.BureauScore,
		BureauScoreScale:     model.BureauScoreScale,
		AdverseRecords:       p.AdverseRecords,
		DebtToIncomeRatio:    p.DebtToIncomeRatio().StringFixed(4),
		ExpenseRatio:         p.ExpenseRatio().StringFixed(4),
		Notes:                p.Notes,
		Source:               string(p.Source),
		CreatedAt:            p.CreatedAt.Format(timeLayout),
		UpdatedAt:            p.UpdatedAt.Format(timeLayout),
	}
}

func ToRiskProfileResponses(profiles []model.RiskProfile) []RiskProfileResponse {
	out := make([]RiskProfileResponse, len(profiles))
	for i := range profiles {
		// Client is only populated when the query preloaded it.
		name := ""
		if profiles[i].Client.ID == profiles[i].ClientID {
			name = profiles[i].Client.FullName()
		}
		out[i] = ToRiskProfileResponse(&profiles[i], name)
	}
	return out
}

func ToRiskFactorResponses(factors []model.RiskFactor) []RiskFactorResponse {
	out := make([]RiskFactorResponse, 0, len(factors))
	for _, f := range factors {
		resp := RiskFactorResponse{
			Code:    f.Code,
			Label:   f.Label,
			Value:   f.Value,
			Impact:  string(f.Impact),
			Points:  f.Points,
			Comment: f.Comment,
		}
		if f.Weight.IsPositive() {
			resp.Weight = f.Weight.StringFixed(4)
		}
		out = append(out, resp)
	}
	return out
}

func ToRiskAssessmentResponse(a *model.RiskAssessment) RiskAssessmentResponse {
	resp := RiskAssessmentResponse{
		ID:                      a.ID.String(),
		ClientID:                a.ClientID.String(),
		RiskProfileID:           a.RiskProfileID.String(),
		RiskModelID:             a.RiskModelID.String(),
		RiskModelName:           a.RiskModelName,
		ModelVersion:            a.ModelVersion,
		Analyzer:                a.Analyzer,
		Score:                   a.Score,
		Grade:                   string(a.Grade),
		ProbabilityOfDefault:    a.ProbabilityOfDefault.StringFixed(4),
		Decision:                string(a.Decision),
		MaxMonthlyPayment:       a.MaxMonthlyPayment.StringFixed(2),
		RecommendedLimit:        a.RecommendedLimit.StringFixed(2),
		RecommendedInstallments: a.RecommendedInstallments,
		RecommendedInterestRate: a.RecommendedInterestRate.StringFixed(4),
		Summary:                 a.Summary,
		PolicyNotes:             a.Notes(),
		Factors:                 ToRiskFactorResponses(a.Factors),
		CreatedAt:               a.CreatedAt.Format(timeLayout),
	}
	if a.Client.ID == a.ClientID {
		resp.ClientName = a.Client.FullName()
	}
	return resp
}

func ToRiskAssessmentResponses(assessments []model.RiskAssessment) []RiskAssessmentResponse {
	out := make([]RiskAssessmentResponse, len(assessments))
	for i := range assessments {
		out[i] = ToRiskAssessmentResponse(&assessments[i])
	}
	return out
}

func ToRiskEvaluationResponse(e *model.RiskEvaluation) RiskEvaluationResponse {
	return RiskEvaluationResponse{
		Score:                   e.Score,
		Grade:                   string(e.Grade),
		ProbabilityOfDefault:    e.ProbabilityOfDefault.StringFixed(4),
		Decision:                string(e.Decision),
		MaxMonthlyPayment:       e.MaxMonthlyPayment.StringFixed(2),
		RecommendedLimit:        e.RecommendedLimit.StringFixed(2),
		RecommendedInstallments: e.RecommendedInstallments,
		RecommendedInterestRate: e.RecommendedInterestRate.StringFixed(4),
		Summary:                 e.Summary,
		PolicyNotes:             e.PolicyNotes,
		Factors:                 ToRiskFactorResponses(e.Factors),
		Analyzer:                e.Analyzer,
	}
}

func ToCreditAvailabilityResponse(a *service.CreditAvailability) CreditAvailabilityResponse {
	resp := CreditAvailabilityResponse{
		ClientID:                a.ClientID.String(),
		Decision:                string(a.Decision),
		RecommendedLimit:        a.RecommendedLimit.StringFixed(2),
		GrantedLimit:            a.GrantedLimit.StringFixed(2),
		UsedAmount:              a.UsedAmount.StringFixed(2),
		AvailableAmount:         a.AvailableAmount.StringFixed(2),
		RecommendedInstallments: a.RecommendedInstallments,
		RecommendedInterestRate: a.RecommendedInterestRate.StringFixed(4),
		Stale:                   a.Stale,
	}
	if a.Assessment != nil {
		assessment := ToRiskAssessmentResponse(a.Assessment)
		resp.Assessment = &assessment
	}
	return resp
}

func ToRiskGradeBandResponses(m *model.RiskModel) []RiskGradeBandResponse {
	out := make([]RiskGradeBandResponse, 0, len(m.Bands))
	for i := range m.Bands {
		b := &m.Bands[i]
		out = append(out, RiskGradeBandResponse{
			Grade:                string(b.Grade),
			MinScore:             b.MinScore,
			InterestSpread:       b.InterestSpread.StringFixed(4),
			ExposureFactor:       b.ExposureFactor.StringFixed(4),
			MaxInstallments:      m.TermCapFor(b),
			Decision:             string(b.Decision),
			ProbabilityOfDefault: b.ProbabilityOfDefault.StringFixed(4),
		})
	}
	return out
}

func ToRiskModelResponse(m *model.RiskModel) RiskModelResponse {
	return RiskModelResponse{
		ID:                      m.ID.String(),
		Name:                    m.Name,
		Version:                 m.Version,
		Prompt:                  m.Prompt,
		Description:             m.Description,
		IsActive:                m.IsActive,
		MinAge:                  m.MinAge,
		MaxAge:                  m.MaxAge,
		MinMonthlyIncome:        m.MinMonthlyIncome.StringFixed(2),
		MaxAdverseRecords:       m.MaxAdverseRecords,
		MaxDTI:                  m.MaxDTI.StringFixed(4),
		MaxPaymentToIncome:      m.MaxPaymentToIncome.StringFixed(4),
		UnverifiedIncomeHaircut: m.UnverifiedIncomeHaircut.StringFixed(4),
		ExpensePerDependent:     m.ExpensePerDependent.StringFixed(2),
		AbsoluteMaxLimit:        m.AbsoluteMaxLimit.StringFixed(2),
		MaxInstallments:         m.MaxInstallments,
		LimitRoundingStep:       m.LimitRoundingStep.StringFixed(2),
		BaseInterestRate:        m.BaseInterestRate.StringFixed(4),
		Bands:                   ToRiskGradeBandResponses(m),
		Placeholders:            model.KnownPromptPlaceholders,
		CreatedAt:               m.CreatedAt.Format(timeLayout),
		UpdatedAt:               m.UpdatedAt.Format(timeLayout),
	}
}

func ToRiskModelResponses(models []model.RiskModel) []RiskModelResponse {
	out := make([]RiskModelResponse, len(models))
	for i := range models {
		out[i] = ToRiskModelResponse(&models[i])
	}
	return out
}

func ToPromptPreviewResponse(p *service.PromptPreview) PromptPreviewResponse {
	baseline := ToRiskEvaluationResponse(&p.Baseline)
	return PromptPreviewResponse{
		Prompt:       p.Prompt,
		Rendered:     p.Rendered,
		Applicant:    p.Applicant,
		Analyzer:     p.Analyzer,
		Placeholders: p.Placeholders,
		Warnings:     p.Warnings,
		Baseline:     &baseline,
	}
}
