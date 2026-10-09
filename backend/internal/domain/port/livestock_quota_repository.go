package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// LivestockQuotaRepository stores livestock quota assessments for audit.
type LivestockQuotaRepository interface {
	Create(ctx context.Context, a *model.LivestockQuotaAssessment) error
}
