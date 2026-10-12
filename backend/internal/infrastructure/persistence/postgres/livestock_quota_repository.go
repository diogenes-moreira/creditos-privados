package postgres

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"gorm.io/gorm"
)

type LivestockQuotaRepository struct {
	db *gorm.DB
}

func NewLivestockQuotaRepository(db *gorm.DB) *LivestockQuotaRepository {
	return &LivestockQuotaRepository{db: db}
}

func (r *LivestockQuotaRepository) Create(ctx context.Context, a *model.LivestockQuotaAssessment) error {
	return r.db.WithContext(ctx).Create(a).Error
}
