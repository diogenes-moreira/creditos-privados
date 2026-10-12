package service

import (
	"context"
	"fmt"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// This file holds the operator-facing half of the risk module: managing the
// analysis model itself. The prompt and the numeric guardrails are data, so an
// operator can retune credit policy without a deployment.

// ActiveModel returns the model new evaluations use, bootstrapping the shipped
// default the first time it is needed so a fresh deployment is never stuck
// without one.
func (s *RiskService) ActiveModel(ctx context.Context) (*model.RiskModel, error) {
	active, err := s.modelRepo.FindActive(ctx)
	if err == nil && active != nil {
		return active, nil
	}
	return s.EnsureDefaultModel(ctx, nil)
}

// EnsureDefaultModel creates and activates the shipped model when no active one
// exists. It is idempotent: with a model already active it returns that one.
func (s *RiskService) EnsureDefaultModel(ctx context.Context, actorID *uuid.UUID) (*model.RiskModel, error) {
	if active, err := s.modelRepo.FindActive(ctx); err == nil && active != nil {
		return active, nil
	}
	m, err := model.DefaultRiskModel(s.country, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to build the default risk model: %w", err)
	}
	if err := s.modelRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to create the default risk model: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_risk_model", "risk_model", m.ID.String(),
		fmt.Sprintf("Default risk model created and activated for %s", s.country))
	return m, nil
}

// ListModels pages through every model version.
func (s *RiskService) ListModels(ctx context.Context, offset, limit int) ([]model.RiskModel, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.modelRepo.FindAll(ctx, offset, limit)
}

// GetModel returns one model version.
func (s *RiskService) GetModel(ctx context.Context, id uuid.UUID) (*model.RiskModel, error) {
	return s.modelRepo.FindByID(ctx, id)
}

// ModelVersions returns the version history of a named model, newest first.
func (s *RiskService) ModelVersions(ctx context.Context, name string) ([]model.RiskModel, error) {
	return s.modelRepo.FindVersions(ctx, name)
}

// CreateModel registers a new, inactive analysis model.
func (s *RiskService) CreateModel(
	ctx context.Context,
	actorID *uuid.UUID,
	name, prompt, description string,
	params model.RiskModelParams,
	bands []model.RiskGradeBand,
) (*model.RiskModel, error) {
	m, err := model.NewRiskModel(name, prompt, description, params, bands, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.modelRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to create risk model: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_risk_model", "risk_model", m.ID.String(),
		fmt.Sprintf("Risk model %s v%d created", m.Name, m.Version))
	return m, nil
}

// SaveModelVersion derives a new version from an existing model and leaves it
// inactive. Editing the prompt this way keeps the running version serving until
// the operator explicitly activates the new one, and preserves the audit trail
// of which version scored any past assessment.
func (s *RiskService) SaveModelVersion(
	ctx context.Context,
	actorID *uuid.UUID,
	sourceID uuid.UUID,
	prompt, description string,
	params model.RiskModelParams,
	bands []model.RiskGradeBand,
	activate bool,
) (*model.RiskModel, error) {
	source, err := s.modelRepo.FindByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	next, err := source.NextVersion(prompt, description, params, bands, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.modelRepo.Create(ctx, next); err != nil {
		return nil, fmt.Errorf("failed to create risk model version: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_risk_model_version", "risk_model", next.ID.String(),
		fmt.Sprintf("Risk model %s v%d created from v%d", next.Name, next.Version, source.Version))

	if activate {
		return s.ActivateModel(ctx, actorID, next.ID)
	}
	return next, nil
}

// UpdateModel edits a model version in place. Use it for corrections to a
// version that is not in service; prefer SaveModelVersion once a version has
// scored assessments so their provenance stays accurate.
func (s *RiskService) UpdateModel(
	ctx context.Context,
	actorID *uuid.UUID,
	id uuid.UUID,
	prompt, description string,
	params model.RiskModelParams,
	bands []model.RiskGradeBand,
) (*model.RiskModel, error) {
	m, err := s.modelRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if prompt != "" {
		if err := m.UpdatePrompt(prompt); err != nil {
			return nil, err
		}
	}
	m.Description = description
	if err := m.UpdateGuardrails(params); err != nil {
		return nil, err
	}

	if len(bands) > 0 {
		previous := m.Bands
		m.Bands = bands
		if err := m.Validate(); err != nil {
			m.Bands = previous
			return nil, err
		}
		if err := s.modelRepo.ReplaceBands(ctx, m.ID, bands); err != nil {
			return nil, fmt.Errorf("failed to update grade bands: %w", err)
		}
	}
	if err := s.modelRepo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to update risk model: %w", err)
	}

	s.audit.Record(ctx, actorID, "update_risk_model", "risk_model", m.ID.String(),
		fmt.Sprintf("Risk model %s v%d updated", m.Name, m.Version))
	return s.modelRepo.FindByID(ctx, m.ID)
}

// ActivateModel makes one version the active analysis model.
func (s *RiskService) ActivateModel(ctx context.Context, actorID *uuid.UUID, id uuid.UUID) (*model.RiskModel, error) {
	m, err := s.modelRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("cannot activate an invalid model: %w", err)
	}
	if err := s.modelRepo.SetActive(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to activate risk model: %w", err)
	}
	s.audit.Record(ctx, actorID, "activate_risk_model", "risk_model", id.String(),
		fmt.Sprintf("Risk model %s v%d activated", m.Name, m.Version))
	return s.modelRepo.FindByID(ctx, id)
}

// PromptPreview is a dry run of a prompt: it validates the placeholders and
// shows the operator exactly what the analysis will receive, using either a
// real client's data or a representative sample when no client is given.
type PromptPreview struct {
	Prompt       string
	Rendered     string
	Applicant    string
	Analyzer     string
	Baseline     model.RiskEvaluation
	Placeholders []string
	Warnings     []string
}

// PreviewPrompt renders a candidate prompt without saving it, so an operator
// can see the effect of an edit before putting it in service.
func (s *RiskService) PreviewPrompt(ctx context.Context, prompt string, clientID *uuid.UUID) (*PromptPreview, error) {
	riskModel, err := s.ActiveModel(ctx)
	if err != nil {
		return nil, err
	}

	candidate := *riskModel
	candidate.Bands = riskModel.Bands
	if prompt != "" {
		if err := candidate.UpdatePrompt(prompt); err != nil {
			return nil, err
		}
	}

	var (
		client  *model.Client
		profile *model.RiskProfile
		history *model.RiskHistory
	)
	preview := &PromptPreview{
		Prompt:       candidate.Prompt,
		Analyzer:     s.AnalyzerName(),
		Placeholders: model.KnownPromptPlaceholders,
	}

	if clientID != nil {
		client, err = s.clientRepo.FindByID(ctx, *clientID)
		if err != nil {
			return nil, fmt.Errorf("client not found: %w", err)
		}
		profile, err = s.profileRepo.FindByClientID(ctx, *clientID)
		if err != nil {
			return nil, fmt.Errorf("client has no risk data on file: %w", err)
		}
		history, err = s.historyRepo.ClientCreditHistory(ctx, *clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to read credit history: %w", err)
		}
	} else {
		client, profile = sampleApplicant(riskModel)
		history = &model.RiskHistory{}
		preview.Warnings = append(preview.Warnings,
			"Rendered with sample data because no client was selected.")
	}

	input := s.buildInput(client, profile, history)
	baseline := candidate.Evaluate(input)
	capacity := candidate.Capacity(input)

	preview.Baseline = baseline
	preview.Rendered = candidate.RenderPrompt(s.promptVars(&candidate, client, profile, input, baseline, capacity))
	preview.Applicant = s.applicantPayload(client, profile, input, capacity)

	if s.analyzer == nil {
		preview.Warnings = append(preview.Warnings,
			"No analysis provider is configured, so evaluations use the deterministic scorecard and this prompt is not sent anywhere.")
	}
	for _, key := range model.KnownPromptPlaceholders {
		if key == "profile" {
			continue
		}
		if !model.UsesPlaceholder(candidate.Prompt, key) {
			preview.Warnings = append(preview.Warnings,
				fmt.Sprintf("The prompt does not use {{%s}}; that context will not reach the analysis.", key))
		}
	}
	return preview, nil
}

// sampleApplicant is a representative applicant used to render a prompt preview
// when the operator has not picked a real client.
func sampleApplicant(m *model.RiskModel) (*model.Client, *model.RiskProfile) {
	income := m.MinMonthlyIncome.Mul(decimal.NewFromInt(3))
	if !income.IsPositive() {
		income = decimal.NewFromInt(500000)
	}
	bureau := 720
	client := &model.Client{
		FirstName:   "Sample",
		LastName:    "Applicant",
		DateOfBirth: time.Now().AddDate(-34, 0, 0),
		City:        "Sample City",
		Province:    "Sample Province",
		Country:     "",
		IVARate:     decimal.NewFromInt(21),
	}
	profile := &model.RiskProfile{
		MonthlyIncome:        income,
		AdditionalIncome:     decimal.Zero,
		IncomeVerified:       true,
		EmploymentType:       model.EmploymentEmployee,
		EmployerName:         "Sample Employer",
		MonthsEmployed:       30,
		MonthlyExpenses:      income.Mul(decimal.NewFromFloat(0.35)).Round(2),
		ExistingDebtPayments: income.Mul(decimal.NewFromFloat(0.08)).Round(2),
		Dependents:           1,
		HousingStatus:        model.HousingRented,
		HasBankAccount:       true,
		BureauScore:          &bureau,
		Source:               model.RiskSourceAPI,
		Notes:                "Sample data for prompt preview.",
	}
	return client, profile
}
