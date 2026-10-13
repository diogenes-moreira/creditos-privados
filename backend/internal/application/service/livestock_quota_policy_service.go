package service

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
)

// LivestockQuotaPolicyService is the operator-facing half of the livestock
// module: it manages the parameters the quota engine runs on. Prices, costs,
// labour thresholds and credit policy are data, so an operator retunes the
// engine without a deployment.
type LivestockQuotaPolicyService struct {
	repo  port.LivestockQuotaPolicyRepository
	audit *AuditService
}

func NewLivestockQuotaPolicyService(repo port.LivestockQuotaPolicyRepository, audit *AuditService) *LivestockQuotaPolicyService {
	return &LivestockQuotaPolicyService{repo: repo, audit: audit}
}

// ActivePolicy returns the policy in service, or nil when none is stored. The
// engine falls back to the shipped defaults in that case, so a deployment that
// has never been tuned still answers.
func (s *LivestockQuotaPolicyService) ActivePolicy(ctx context.Context) *model.LivestockQuotaPolicy {
	if s == nil || s.repo == nil {
		return nil
	}
	active, err := s.repo.FindActive(ctx)
	if err != nil || active == nil {
		return nil
	}
	return active
}

// ActiveParams returns the parameters the engine must use: the active policy's
// when one exists, the shipped defaults otherwise.
func (s *LivestockQuotaPolicyService) ActiveParams(ctx context.Context) model.QuotaParams {
	if active := s.ActivePolicy(ctx); active != nil {
		return active.Params()
	}
	return model.DefaultQuotaParams()
}

// EnsureDefaultPolicy stores and activates the shipped parameters when nothing
// is active yet, so the admin screen always has a policy to edit. It is
// idempotent: with a policy already active it returns that one.
func (s *LivestockQuotaPolicyService) EnsureDefaultPolicy(ctx context.Context, actorID *uuid.UUID) (*model.LivestockQuotaPolicy, error) {
	if active := s.ActivePolicy(ctx); active != nil {
		return active, nil
	}
	p, err := model.DefaultLivestockQuotaPolicy(actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to build the default livestock quota policy: %w", err)
	}
	p.Activate()
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("failed to create the default livestock quota policy: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_livestock_quota_policy", "livestock_quota_policy", p.ID.String(),
		fmt.Sprintf("Default livestock quota policy %s v%d created and activated", p.Name, p.Version))
	return s.repo.FindByID(ctx, p.ID)
}

// List pages through every policy version.
func (s *LivestockQuotaPolicyService) List(ctx context.Context, offset, limit int) ([]model.LivestockQuotaPolicy, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.FindAll(ctx, offset, limit)
}

// Get returns one policy version.
func (s *LivestockQuotaPolicyService) Get(ctx context.Context, id uuid.UUID) (*model.LivestockQuotaPolicy, error) {
	return s.repo.FindByID(ctx, id)
}

// Create registers a new, inactive parameter set, activating it when asked.
func (s *LivestockQuotaPolicyService) Create(
	ctx context.Context,
	actorID *uuid.UUID,
	name, description string,
	params model.QuotaParams,
	activate bool,
) (*model.LivestockQuotaPolicy, error) {
	p, err := model.NewLivestockQuotaPolicy(name, description, params, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("failed to create livestock quota policy: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_livestock_quota_policy", "livestock_quota_policy", p.ID.String(),
		fmt.Sprintf("Livestock quota policy %s v%d created", p.Name, p.Version))

	if activate {
		return s.Activate(ctx, actorID, p.ID)
	}
	return s.repo.FindByID(ctx, p.ID)
}

// SaveVersion derives a new version from an existing policy and leaves it
// inactive unless asked otherwise. This is the normal way an operator edits the
// parameters: the running version keeps serving every quota request until the
// new one is activated, and past assessments stay explainable.
func (s *LivestockQuotaPolicyService) SaveVersion(
	ctx context.Context,
	actorID *uuid.UUID,
	sourceID uuid.UUID,
	description string,
	params model.QuotaParams,
	activate bool,
) (*model.LivestockQuotaPolicy, error) {
	source, err := s.repo.FindByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	next, err := source.NextVersion(description, params, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, next); err != nil {
		return nil, fmt.Errorf("failed to create livestock quota policy version: %w", err)
	}
	s.audit.Record(ctx, actorID, "create_livestock_quota_policy_version", "livestock_quota_policy", next.ID.String(),
		fmt.Sprintf("Livestock quota policy %s v%d created from v%d", next.Name, next.Version, source.Version))

	if activate {
		return s.Activate(ctx, actorID, next.ID)
	}
	return s.repo.FindByID(ctx, next.ID)
}

// Update edits a policy version in place. Use it to correct a version that is
// not in service; prefer SaveVersion once a version has priced quotas.
func (s *LivestockQuotaPolicyService) Update(
	ctx context.Context,
	actorID *uuid.UUID,
	id uuid.UUID,
	description string,
	params model.QuotaParams,
) (*model.LivestockQuotaPolicy, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Description = description
	if err := p.UpdateParams(params); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceFactors(ctx, p.ID, p.Factors); err != nil {
		return nil, fmt.Errorf("failed to update the UGG factor matrix: %w", err)
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("failed to update livestock quota policy: %w", err)
	}
	s.audit.Record(ctx, actorID, "update_livestock_quota_policy", "livestock_quota_policy", p.ID.String(),
		fmt.Sprintf("Livestock quota policy %s v%d updated", p.Name, p.Version))
	return s.repo.FindByID(ctx, p.ID)
}

// Activate makes one version the parameters every new quota is computed with.
func (s *LivestockQuotaPolicyService) Activate(ctx context.Context, actorID *uuid.UUID, id uuid.UUID) (*model.LivestockQuotaPolicy, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("cannot activate an invalid livestock quota policy: %w", err)
	}
	if err := s.repo.SetActive(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to activate livestock quota policy: %w", err)
	}
	s.audit.Record(ctx, actorID, "activate_livestock_quota_policy", "livestock_quota_policy", id.String(),
		fmt.Sprintf("Livestock quota policy %s v%d activated", p.Name, p.Version))
	return s.repo.FindByID(ctx, id)
}
