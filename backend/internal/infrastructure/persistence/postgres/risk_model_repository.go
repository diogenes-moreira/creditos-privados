package postgres

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RiskModelRepository struct {
	db *gorm.DB
}

func NewRiskModelRepository(db *gorm.DB) *RiskModelRepository {
	return &RiskModelRepository{db: db}
}

func (r *RiskModelRepository) Create(ctx context.Context, m *model.RiskModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *RiskModelRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.RiskModel, error) {
	var m model.RiskModel
	if err := r.db.WithContext(ctx).Preload("Bands").First(&m, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("risk model not found: %w", err)
	}
	return &m, nil
}

func (r *RiskModelRepository) FindActive(ctx context.Context) (*model.RiskModel, error) {
	var m model.RiskModel
	if err := r.db.WithContext(ctx).Preload("Bands").
		Where("is_active = ?", true).
		Order("version DESC").First(&m).Error; err != nil {
		return nil, fmt.Errorf("no active risk model: %w", err)
	}
	return &m, nil
}

func (r *RiskModelRepository) FindAll(ctx context.Context, offset, limit int) ([]model.RiskModel, int64, error) {
	var models []model.RiskModel
	var total int64
	base := r.db.WithContext(ctx).Model(&model.RiskModel{})
	base.Count(&total)
	if err := base.Preload("Bands").Offset(offset).Limit(limit).
		Order("name ASC, version DESC").Find(&models).Error; err != nil {
		return nil, 0, err
	}
	return models, total, nil
}

func (r *RiskModelRepository) FindVersions(ctx context.Context, name string) ([]model.RiskModel, error) {
	var models []model.RiskModel
	if err := r.db.WithContext(ctx).Preload("Bands").
		Where("name = ?", name).Order("version DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (r *RiskModelRepository) Update(ctx context.Context, m *model.RiskModel) error {
	// Save would cascade-upsert the preloaded bands; band changes go through
	// ReplaceBands so an edit to the model itself cannot silently duplicate them.
	return r.db.WithContext(ctx).Omit("Bands").Save(m).Error
}

func (r *RiskModelRepository) ReplaceBands(ctx context.Context, modelID uuid.UUID, bands []model.RiskGradeBand) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("risk_model_id = ?", modelID).Delete(&model.RiskGradeBand{}).Error; err != nil {
			return err
		}
		if len(bands) == 0 {
			return nil
		}
		for i := range bands {
			bands[i].RiskModelID = modelID
			if bands[i].ID == uuid.Nil {
				bands[i].ID = uuid.New()
			}
		}
		return tx.Create(&bands).Error
	})
}

func (r *RiskModelRepository) SetActive(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.RiskModel{}).
			Where("is_active = ?", true).
			Update("is_active", false).Error; err != nil {
			return err
		}
		res := tx.Model(&model.RiskModel{}).Where("id = ?", id).Update("is_active", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("risk model %s not found", id)
		}
		return nil
	})
}
