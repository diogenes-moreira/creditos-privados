package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
)

// LivestockCreditLineRepository stores ranchers' livestock credit lines and
// their signed promissory notes. The finders return nil, nil when nothing
// matches.
type LivestockCreditLineRepository interface {
	// Create stores the line and its document together, or neither.
	Create(ctx context.Context, line *model.LivestockCreditLine, doc *model.LivestockCreditLineDocument) error
	FindByPromissoryNote(ctx context.Context, number string) (*model.LivestockCreditLine, error)
	FindByPartnerReference(ctx context.Context, partner, reference string) (*model.LivestockCreditLine, error)
	FindByAssessment(ctx context.Context, assessmentID uuid.UUID) (*model.LivestockCreditLine, error)
}
