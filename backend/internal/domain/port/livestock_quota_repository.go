package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
)

// LivestockQuotaRepository stores livestock quota assessments for audit.
type LivestockQuotaRepository interface {
	Create(ctx context.Context, a *model.LivestockQuotaAssessment) error
}

// LivestockQuotaPolicyRepository stores the versioned parameter sets the
// livestock quota engine runs on. Exactly one policy is active at a time.
type LivestockQuotaPolicyRepository interface {
	Create(ctx context.Context, p *model.LivestockQuotaPolicy) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.LivestockQuotaPolicy, error)
	FindActive(ctx context.Context) (*model.LivestockQuotaPolicy, error)
	FindAll(ctx context.Context, offset, limit int) ([]model.LivestockQuotaPolicy, int64, error)
	Update(ctx context.Context, p *model.LivestockQuotaPolicy) error
	// ReplaceFactors swaps a policy's UGG matrix atomically.
	ReplaceFactors(ctx context.Context, policyID uuid.UUID, factors []model.LivestockUGGFactor) error
	// SetActive activates one policy and deactivates every other one.
	SetActive(ctx context.Context, id uuid.UUID) error
}
