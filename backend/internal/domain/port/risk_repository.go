package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
)

type RiskProfileRepository interface {
	Create(ctx context.Context, profile *model.RiskProfile) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.RiskProfile, error)
	FindByClientID(ctx context.Context, clientID uuid.UUID) (*model.RiskProfile, error)
	FindAll(ctx context.Context, offset, limit int) ([]model.RiskProfile, int64, error)
	Update(ctx context.Context, profile *model.RiskProfile) error
}

type RiskModelRepository interface {
	Create(ctx context.Context, m *model.RiskModel) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.RiskModel, error)
	FindActive(ctx context.Context) (*model.RiskModel, error)
	FindAll(ctx context.Context, offset, limit int) ([]model.RiskModel, int64, error)
	FindVersions(ctx context.Context, name string) ([]model.RiskModel, error)
	Update(ctx context.Context, m *model.RiskModel) error
	// ReplaceBands swaps a model's grade bands atomically.
	ReplaceBands(ctx context.Context, modelID uuid.UUID, bands []model.RiskGradeBand) error
	// SetActive activates one model and deactivates every other one.
	SetActive(ctx context.Context, id uuid.UUID) error
}

type RiskAssessmentRepository interface {
	Create(ctx context.Context, a *model.RiskAssessment) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.RiskAssessment, error)
	FindLatestByClientID(ctx context.Context, clientID uuid.UUID) (*model.RiskAssessment, error)
	FindByClientID(ctx context.Context, clientID uuid.UUID, offset, limit int) ([]model.RiskAssessment, int64, error)
	FindByDecision(ctx context.Context, decision model.RiskDecision, offset, limit int) ([]model.RiskAssessment, int64, error)
}

// CreditHistoryRepository reads the repayment behaviour the risk analysis needs.
// It is a read-only projection over loans, installments and payments.
type CreditHistoryRepository interface {
	ClientCreditHistory(ctx context.Context, clientID uuid.UUID) (*model.RiskHistory, error)
}
