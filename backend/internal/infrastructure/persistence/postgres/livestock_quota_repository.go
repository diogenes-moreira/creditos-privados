package postgres

import (
	"context"
	"errors"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
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

func (r *LivestockQuotaRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.LivestockQuotaAssessment, error) {
	var a model.LivestockQuotaAssessment
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
