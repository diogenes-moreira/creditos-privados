package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RiskService owns the credit-risk module: it ingests applicant risk data, runs
// the operator-managed analysis model over it, and turns the result into a
// credit availability figure the rest of the system can act on.
type RiskService struct {
	profileRepo    port.RiskProfileRepository
	modelRepo      port.RiskModelRepository
	assessmentRepo port.RiskAssessmentRepository
	historyRepo    port.CreditHistoryRepository
	clientRepo     port.ClientRepository
	creditLineRepo port.CreditLineRepository
	creditService  *CreditService
	audit          *AuditService

	// analyzer is optional. When nil, the deterministic scorecard is the only
	// analysis engine, which keeps the module fully functional with no AI
	// credentials configured.
	analyzer port.RiskAnalyzer

	country        string
	currency       string
	defaultIVARate decimal.Decimal
}

func NewRiskService(
	profileRepo port.RiskProfileRepository,
	modelRepo port.RiskModelRepository,
	assessmentRepo port.RiskAssessmentRepository,
	historyRepo port.CreditHistoryRepository,
	clientRepo port.ClientRepository,
	creditLineRepo port.CreditLineRepository,
	creditService *CreditService,
	audit *AuditService,
	analyzer port.RiskAnalyzer,
	country, currency string,
	defaultIVARate decimal.Decimal,
) *RiskService {
	return &RiskService{
		profileRepo:    profileRepo,
		modelRepo:      modelRepo,
		assessmentRepo: assessmentRepo,
		historyRepo:    historyRepo,
		clientRepo:     clientRepo,
		creditLineRepo: creditLineRepo,
		creditService:  creditService,
		audit:          audit,
		analyzer:       analyzer,
		country:        country,
		currency:       currency,
		defaultIVARate: defaultIVARate,
	}
}

// AnalyzerName reports which engine will run the analysis.
func (s *RiskService) AnalyzerName() string {
	if s.analyzer == nil {
		return model.AnalyzerScorecard
	}
	return s.analyzer.Name()
}

// ---------------------------------------------------------------- risk data

// SubmitProfile creates or replaces a client's risk data. It is the single
// ingestion point for every channel: the client portal, an operator, and the
// public API all land here.
func (s *RiskService) SubmitProfile(
	ctx context.Context,
	actorID *uuid.UUID,
	clientID uuid.UUID,
	in model.RiskProfileInput,
	source model.RiskDataSource,
) (*model.RiskProfile, error) {
	if _, err := s.clientRepo.FindByID(ctx, clientID); err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	existing, err := s.profileRepo.FindByClientID(ctx, clientID)
	if err == nil && existing != nil {
		// A self-service submission may not overwrite the lender's own
		// attestations, so those fields are carried forward from the record.
		if err := existing.Update(in.RestrictToSource(source, existing), source, actorID); err != nil {
			return nil, err
		}
		if err := s.profileRepo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update risk profile: %w", err)
		}
		s.audit.Record(ctx, actorID, "update_risk_profile", "risk_profile", existing.ID.String(),
			fmt.Sprintf("Risk data updated via %s for client %s", source, clientID))
		return existing, nil
	}

	profile, err := model.NewRiskProfile(clientID, in.RestrictToSource(source, nil), source, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.profileRepo.Create(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to create risk profile: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_risk_profile", "risk_profile", profile.ID.String(),
		fmt.Sprintf("Risk data submitted via %s for client %s", source, clientID))
	return profile, nil
}

// GetProfile returns a client's risk data.
func (s *RiskService) GetProfile(ctx context.Context, clientID uuid.UUID) (*model.RiskProfile, error) {
	return s.profileRepo.FindByClientID(ctx, clientID)
}

// ListProfiles pages through all submitted risk data.
func (s *RiskService) ListProfiles(ctx context.Context, offset, limit int) ([]model.RiskProfile, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.profileRepo.FindAll(ctx, offset, limit)
}

// ---------------------------------------------------------------- analysis

// Evaluate runs the active analysis model over a client's risk data and stores
// the resulting assessment.
func (s *RiskService) Evaluate(ctx context.Context, actorID *uuid.UUID, clientID uuid.UUID) (*model.RiskAssessment, error) {
	client, err := s.clientRepo.FindByID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}
	profile, err := s.profileRepo.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client has no risk data on file: %w", err)
	}
	riskModel, err := s.ActiveModel(ctx)
	if err != nil {
		return nil, err
	}
	history, err := s.historyRepo.ClientCreditHistory(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to read credit history: %w", err)
	}

	input := s.buildInput(client, profile, history)
	eval := s.runAnalysis(ctx, riskModel, client, profile, input)

	assessment, err := model.NewRiskAssessment(clientID, profile, riskModel, eval, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.assessmentRepo.Create(ctx, assessment); err != nil {
		return nil, fmt.Errorf("failed to store risk assessment: %w", err)
	}

	s.audit.Record(ctx, actorID, "evaluate_risk", "risk_assessment", assessment.ID.String(),
		fmt.Sprintf("Risk evaluated with %s v%d (%s): score %d, grade %s, %s, limit %s",
			riskModel.Name, riskModel.Version, eval.Analyzer, assessment.Score,
			assessment.Grade, assessment.Decision, assessment.RecommendedLimit.StringFixed(2)))
	return assessment, nil
}

// Simulate scores hypothetical risk data without touching any client record.
// It backs pre-qualification: an applicant can see what they would qualify for
// before an account exists.
func (s *RiskService) Simulate(ctx context.Context, in model.RiskProfileInput, age int) (*model.RiskEvaluation, error) {
	riskModel, err := s.ActiveModel(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := model.NewRiskProfile(uuid.Nil, in, model.RiskSourceAPI, nil)
	if err != nil {
		return nil, err
	}
	input := model.RiskInput{
		Profile: profile,
		Age:     age,
		IVARate: s.defaultIVARate,
		History: model.RiskHistory{
			CurrentOverdueAmount: decimal.Zero,
			OutstandingBalance:   decimal.Zero,
		},
	}
	// A simulation is always scored by the deterministic scorecard: it must be
	// reproducible and free, and there is no client to bill an analysis to.
	eval := riskModel.Evaluate(input)
	return &eval, nil
}

// runAnalysis prefers the prompt-driven analyzer and falls back to the
// scorecard when none is configured or the call fails, so an evaluation always
// produces a usable answer.
func (s *RiskService) runAnalysis(
	ctx context.Context,
	riskModel *model.RiskModel,
	client *model.Client,
	profile *model.RiskProfile,
	input model.RiskInput,
) model.RiskEvaluation {
	baseline := riskModel.Evaluate(input)
	if s.analyzer == nil {
		return baseline
	}

	capacity := riskModel.Capacity(input)
	prompt := riskModel.RenderPrompt(s.promptVars(riskModel, client, profile, input, baseline, capacity))
	applicant := s.applicantPayload(client, profile, input, capacity)

	result, err := s.analyzer.Analyze(ctx, port.RiskAnalysisRequest{
		Prompt:    prompt,
		Applicant: applicant,
		Baseline:  baseline,
		Capacity:  capacity,
	})
	if err != nil {
		// A failed analysis must not block underwriting; fall back to the
		// scorecard and record why on the assessment.
		log.Printf("risk analysis via %s failed, falling back to scorecard: %v", s.analyzer.Name(), err)
		baseline.AddNote(fmt.Sprintf("analysis via %s unavailable (%v); scored by the deterministic scorecard", s.analyzer.Name(), err))
		return baseline
	}

	eval := result.ToEvaluation(model.AnalyzerLLM)
	riskModel.ApplyGuardrails(&eval, input, capacity)
	eval.AddNote(fmt.Sprintf("analysed by %s using %s v%d", s.analyzer.Name(), riskModel.Name, riskModel.Version))
	if len(eval.Factors) == 0 {
		eval.Factors = baseline.Factors
		eval.AddNote("analyzer returned no factors; scorecard factors retained for explainability")
	}
	return eval
}

func (s *RiskService) buildInput(client *model.Client, profile *model.RiskProfile, history *model.RiskHistory) model.RiskInput {
	ivaRate := client.IVARate
	if !ivaRate.IsPositive() {
		ivaRate = s.defaultIVARate
	}
	in := model.RiskInput{
		Profile:   profile,
		Age:       ageAt(client.DateOfBirth, time.Now()),
		IsBlocked: client.IsBlocked,
		IVARate:   ivaRate,
	}
	if history != nil {
		in.History = *history
	}
	return in
}

// ageAt returns the completed years between a date of birth and a reference
// date, counting the birthday itself as the day the age increments.
func ageAt(dob, at time.Time) int {
	age := at.Year() - dob.Year()
	if at.Month() < dob.Month() || (at.Month() == dob.Month() && at.Day() < dob.Day()) {
		age--
	}
	if age < 0 {
		return 0
	}
	return age
}

// promptVars supplies the {{placeholder}} values the operator's prompt uses.
func (s *RiskService) promptVars(
	riskModel *model.RiskModel,
	client *model.Client,
	profile *model.RiskProfile,
	input model.RiskInput,
	baseline model.RiskEvaluation,
	capacity model.RiskCapacity,
) map[string]string {
	return map[string]string{
		"client":   toJSON(clientView(client, input.Age)),
		"profile":  toJSON(profileView(profile)),
		"history":  toJSON(historyView(input.History)),
		"policy":   toJSON(policyView(riskModel, capacity)),
		"baseline": toJSON(baselineView(baseline)),
		"currency": s.currency,
		"country":  s.country,
		"today":    time.Now().Format("2006-01-02"),
	}
}

// applicantPayload is the user-turn message: the same applicant data the prompt
// already embeds, sent as one document so the model has it in a single place
// even if the operator removes a placeholder from the prompt.
func (s *RiskService) applicantPayload(
	client *model.Client,
	profile *model.RiskProfile,
	input model.RiskInput,
	capacity model.RiskCapacity,
) string {
	return toJSON(map[string]any{
		"client":   clientView(client, input.Age),
		"riskData": profileView(profile),
		"history":  historyView(input.History),
		"affordability": map[string]string{
			"recognizedIncome":  capacity.RecognizedIncome.StringFixed(2),
			"disposableIncome":  capacity.DisposableIncome.StringFixed(2),
			"maxMonthlyPayment": capacity.MaxMonthlyPayment.StringFixed(2),
			"maxLimit":          capacity.MaxLimit.StringFixed(2),
		},
		"currency": s.currency,
	})
}

func clientView(client *model.Client, age int) map[string]any {
	return map[string]any{
		"name":     client.FullName(),
		"age":      age,
		"city":     client.City,
		"province": client.Province,
		"country":  client.Country,
		"isPEP":    client.IsPEP,
		"blocked":  client.IsBlocked,
		"comments": client.Comments,
	}
}

func profileView(p *model.RiskProfile) map[string]any {
	view := map[string]any{
		"monthlyIncome":        p.MonthlyIncome.StringFixed(2),
		"additionalIncome":     p.AdditionalIncome.StringFixed(2),
		"incomeVerified":       p.IncomeVerified,
		"employmentType":       string(p.EmploymentType),
		"employerName":         p.EmployerName,
		"monthsEmployed":       p.MonthsEmployed,
		"monthlyExpenses":      p.MonthlyExpenses.StringFixed(2),
		"existingDebtPayments": p.ExistingDebtPayments.StringFixed(2),
		"dependents":           p.Dependents,
		"housingStatus":        string(p.HousingStatus),
		"hasBankAccount":       p.HasBankAccount,
		"adverseRecords":       p.AdverseRecords,
		"debtToIncomeRatio":    p.DebtToIncomeRatio().StringFixed(4),
		"expenseRatio":         p.ExpenseRatio().StringFixed(4),
		"notes":                p.Notes,
		"source":               string(p.Source),
	}
	if p.HasBureauScore() {
		view["bureauScore"] = *p.BureauScore
		view["bureauScoreScale"] = model.BureauScoreScale
	} else {
		view["bureauScore"] = nil
	}
	return view
}

func historyView(h model.RiskHistory) map[string]any {
	return map[string]any{
		"loansTotal":             h.LoansTotal,
		"loansActive":            h.LoansActive,
		"installmentsDue":        h.InstallmentsDue,
		"installmentsPaidOnTime": h.InstallmentsPaidOnTime,
		"installmentsLate":       h.InstallmentsLate,
		"maxDaysLate":            h.MaxDaysLate,
		"onTimeRate":             h.OnTimeRate().StringFixed(4),
		"currentOverdueAmount":   h.CurrentOverdueAmount.StringFixed(2),
		"outstandingBalance":     h.OutstandingBalance.StringFixed(2),
	}
}

func policyView(m *model.RiskModel, capacity model.RiskCapacity) map[string]any {
	bands := make([]map[string]any, 0, len(m.Bands))
	for _, b := range m.Bands {
		bands = append(bands, map[string]any{
			"grade":                string(b.Grade),
			"minScore":             b.MinScore,
			"decision":             string(b.Decision),
			"interestSpread":       b.InterestSpread.StringFixed(4),
			"exposureFactor":       b.ExposureFactor.StringFixed(4),
			"maxInstallments":      m.TermCapFor(&b),
			"probabilityOfDefault": b.ProbabilityOfDefault.StringFixed(4),
		})
	}
	return map[string]any{
		"minAge":                  m.MinAge,
		"maxAge":                  m.MaxAge,
		"minMonthlyIncome":        m.MinMonthlyIncome.StringFixed(2),
		"maxAdverseRecords":       m.MaxAdverseRecords,
		"maxDebtToIncome":         m.MaxDTI.StringFixed(4),
		"maxPaymentToIncome":      m.MaxPaymentToIncome.StringFixed(4),
		"unverifiedIncomeHaircut": m.UnverifiedIncomeHaircut.StringFixed(4),
		"expensePerDependent":     m.ExpensePerDependent.StringFixed(2),
		"absoluteMaxLimit":        m.AbsoluteMaxLimit.StringFixed(2),
		"maxInstallments":         m.MaxInstallments,
		"baseInterestRate":        m.BaseInterestRate.StringFixed(4),
		"grades":                  bands,
		"affordabilityCap": map[string]string{
			"maxMonthlyPayment": capacity.MaxMonthlyPayment.StringFixed(2),
			"maxLimit":          capacity.MaxLimit.StringFixed(2),
		},
	}
}

func baselineView(e model.RiskEvaluation) map[string]any {
	factors := make([]map[string]any, 0, len(e.Factors))
	for _, f := range e.Factors {
		factors = append(factors, map[string]any{
			"code":   f.Code,
			"label":  f.Label,
			"value":  f.Value,
			"points": f.Points,
			"weight": f.Weight.StringFixed(4),
		})
	}
	return map[string]any{
		"score":                   e.Score,
		"grade":                   string(e.Grade),
		"decision":                string(e.Decision),
		"maxMonthlyPayment":       e.MaxMonthlyPayment.StringFixed(2),
		"recommendedLimit":        e.RecommendedLimit.StringFixed(2),
		"recommendedInstallments": e.RecommendedInstallments,
		"recommendedInterestRate": e.RecommendedInterestRate.StringFixed(4),
		"factors":                 factors,
	}
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------- results

// LatestAssessment returns the most recent assessment for a client.
func (s *RiskService) LatestAssessment(ctx context.Context, clientID uuid.UUID) (*model.RiskAssessment, error) {
	return s.assessmentRepo.FindLatestByClientID(ctx, clientID)
}

// ListAssessments pages through a client's assessment history.
func (s *RiskService) ListAssessments(ctx context.Context, clientID uuid.UUID, offset, limit int) ([]model.RiskAssessment, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.assessmentRepo.FindByClientID(ctx, clientID, offset, limit)
}

// PendingReviews lists assessments waiting on a human decision.
func (s *RiskService) PendingReviews(ctx context.Context, offset, limit int) ([]model.RiskAssessment, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.assessmentRepo.FindByDecision(ctx, model.RiskDecisionManualReview, offset, limit)
}

// CreditAvailability is what the risk module answers: how much credit a client
// can be given, and how much of it is still unused.
type CreditAvailability struct {
	ClientID                uuid.UUID
	Assessment              *model.RiskAssessment
	RecommendedLimit        decimal.Decimal
	GrantedLimit            decimal.Decimal
	UsedAmount              decimal.Decimal
	AvailableAmount         decimal.Decimal
	RecommendedInstallments int
	RecommendedInterestRate decimal.Decimal
	Decision                model.RiskDecision
	Stale                   bool
}

// AvailableCredit combines the latest assessment with the client's existing
// exposure. Granted and recommended limits are reported separately so an
// operator can see when the portfolio has drifted from what risk recommends.
func (s *RiskService) AvailableCredit(ctx context.Context, clientID uuid.UUID) (*CreditAvailability, error) {
	assessment, err := s.assessmentRepo.FindLatestByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client has no risk assessment: %w", err)
	}

	lines, err := s.creditLineRepo.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to read credit lines: %w", err)
	}

	granted := decimal.Zero
	used := decimal.Zero
	for _, cl := range lines {
		if cl.Status != model.CreditLineApproved {
			continue
		}
		granted = granted.Add(cl.MaxAmount)
		used = used.Add(cl.UsedAmount)
	}

	availability := &CreditAvailability{
		ClientID:                clientID,
		Assessment:              assessment,
		RecommendedLimit:        assessment.RecommendedLimit,
		GrantedLimit:            granted,
		UsedAmount:              used,
		AvailableAmount:         assessment.AvailableCredit(used),
		RecommendedInstallments: assessment.RecommendedInstallments,
		RecommendedInterestRate: assessment.RecommendedInterestRate,
		Decision:                assessment.Decision,
	}

	// An assessment older than the current risk data no longer describes the
	// applicant, so flag it rather than presenting it as current.
	if profile, err := s.profileRepo.FindByClientID(ctx, clientID); err == nil {
		availability.Stale = profile.UpdatedAt.After(assessment.CreatedAt)
	}
	return availability, nil
}

// ApplyRecommendation writes the assessed limit onto the client's credit line,
// creating one when none exists. It refuses to act on anything the analysis did
// not approve, so a manual-review or declined case still needs a human.
func (s *RiskService) ApplyRecommendation(ctx context.Context, adminID, clientID uuid.UUID) (*model.CreditLine, error) {
	assessment, err := s.assessmentRepo.FindLatestByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client has no risk assessment: %w", err)
	}
	if !assessment.IsApproved() {
		return nil, fmt.Errorf("cannot apply a recommendation with decision %s", assessment.Decision)
	}
	if !assessment.RecommendedLimit.IsPositive() {
		return nil, fmt.Errorf("assessment recommends no credit")
	}

	lines, err := s.creditLineRepo.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to read credit lines: %w", err)
	}

	if len(lines) == 0 {
		cl, err := s.creditService.CreateCreditLine(
			ctx, adminID, clientID,
			assessment.RecommendedLimit,
			assessment.RecommendedInterestRate,
			assessment.RecommendedInstallments,
			false,
		)
		if err != nil {
			return nil, err
		}
		s.audit.Record(ctx, &adminID, "apply_risk_recommendation", "credit_line", cl.ID.String(),
			fmt.Sprintf("Credit line created from risk assessment %s: limit %s, %d months at %s",
				assessment.ID, assessment.RecommendedLimit.StringFixed(2),
				assessment.RecommendedInstallments, assessment.RecommendedInterestRate.StringFixed(4)))
		return cl, nil
	}

	target := &lines[0]
	previous := target.MaxAmount
	cl, err := s.creditService.UpdateCreditLineMaxAmount(ctx, adminID, target.ID, assessment.RecommendedLimit)
	if err != nil {
		return nil, err
	}
	cl.InterestRate = assessment.RecommendedInterestRate
	cl.MaxInstallments = assessment.RecommendedInstallments
	if err := s.creditService.UpdateCreditLineFields(ctx, cl); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, &adminID, "apply_risk_recommendation", "credit_line", cl.ID.String(),
		fmt.Sprintf("Credit line updated from risk assessment %s: limit %s -> %s, %d months at %s",
			assessment.ID, previous.StringFixed(2), assessment.RecommendedLimit.StringFixed(2),
			assessment.RecommendedInstallments, assessment.RecommendedInterestRate.StringFixed(4)))
	return cl, nil
}
