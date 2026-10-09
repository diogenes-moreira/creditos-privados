package postgres

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RiskAssessmentRepository struct {
	db *gorm.DB
}

func NewRiskAssessmentRepository(db *gorm.DB) *RiskAssessmentRepository {
	return &RiskAssessmentRepository{db: db}
}

func (r *RiskAssessmentRepository) Create(ctx context.Context, a *model.RiskAssessment) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *RiskAssessmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.RiskAssessment, error) {
	var a model.RiskAssessment
	if err := r.db.WithContext(ctx).Preload("Factors").First(&a, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("risk assessment not found: %w", err)
	}
	return &a, nil
}

func (r *RiskAssessmentRepository) FindLatestByClientID(ctx context.Context, clientID uuid.UUID) (*model.RiskAssessment, error) {
	var a model.RiskAssessment
	if err := r.db.WithContext(ctx).Preload("Factors").
		Where("client_id = ?", clientID).
		Order("created_at DESC").First(&a).Error; err != nil {
		return nil, fmt.Errorf("no risk assessment for client: %w", err)
	}
	return &a, nil
}

func (r *RiskAssessmentRepository) FindByClientID(ctx context.Context, clientID uuid.UUID, offset, limit int) ([]model.RiskAssessment, int64, error) {
	var assessments []model.RiskAssessment
	var total int64
	base := r.db.WithContext(ctx).Model(&model.RiskAssessment{}).Where("client_id = ?", clientID)
	base.Count(&total)
	if err := base.Preload("Factors").Offset(offset).Limit(limit).
		Order("created_at DESC").Find(&assessments).Error; err != nil {
		return nil, 0, err
	}
	return assessments, total, nil
}

func (r *RiskAssessmentRepository) FindByDecision(ctx context.Context, decision model.RiskDecision, offset, limit int) ([]model.RiskAssessment, int64, error) {
	var assessments []model.RiskAssessment
	var total int64
	base := r.db.WithContext(ctx).Model(&model.RiskAssessment{}).Where("decision = ?", decision)
	base.Count(&total)
	if err := base.Preload("Client").Preload("Factors").Offset(offset).Limit(limit).
		Order("created_at DESC").Find(&assessments).Error; err != nil {
		return nil, 0, err
	}
	return assessments, total, nil
}
