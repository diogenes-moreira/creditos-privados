package postgres

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RiskProfileRepository struct {
	db *gorm.DB
}

func NewRiskProfileRepository(db *gorm.DB) *RiskProfileRepository {
	return &RiskProfileRepository{db: db}
}

func (r *RiskProfileRepository) Create(ctx context.Context, profile *model.RiskProfile) error {
	return r.db.WithContext(ctx).Create(profile).Error
}

func (r *RiskProfileRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.RiskProfile, error) {
	var p model.RiskProfile
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("risk profile not found: %w", err)
	}
	return &p, nil
}

func (r *RiskProfileRepository) FindByClientID(ctx context.Context, clientID uuid.UUID) (*model.RiskProfile, error) {
	var p model.RiskProfile
	if err := r.db.WithContext(ctx).First(&p, "client_id = ?", clientID).Error; err != nil {
		return nil, fmt.Errorf("risk profile not found: %w", err)
	}
	return &p, nil
}

func (r *RiskProfileRepository) FindAll(ctx context.Context, offset, limit int) ([]model.RiskProfile, int64, error) {
	var profiles []model.RiskProfile
	var total int64
	base := r.db.WithContext(ctx).Model(&model.RiskProfile{})
	base.Count(&total)
	if err := base.Preload("Client").Offset(offset).Limit(limit).Order("updated_at DESC").Find(&profiles).Error; err != nil {
		return nil, 0, err
	}
	return profiles, total, nil
}

func (r *RiskProfileRepository) Update(ctx context.Context, profile *model.RiskProfile) error {
	return r.db.WithContext(ctx).Save(profile).Error
}
