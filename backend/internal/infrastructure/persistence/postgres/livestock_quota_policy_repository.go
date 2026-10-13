package postgres

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LivestockQuotaPolicyRepository struct {
	db *gorm.DB
}

func NewLivestockQuotaPolicyRepository(db *gorm.DB) *LivestockQuotaPolicyRepository {
	return &LivestockQuotaPolicyRepository{db: db}
}

func (r *LivestockQuotaPolicyRepository) Create(ctx context.Context, p *model.LivestockQuotaPolicy) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *LivestockQuotaPolicyRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.LivestockQuotaPolicy, error) {
	var p model.LivestockQuotaPolicy
	if err := r.db.WithContext(ctx).Preload("Factors").First(&p, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("livestock quota policy not found: %w", err)
	}
	return &p, nil
}

func (r *LivestockQuotaPolicyRepository) FindActive(ctx context.Context) (*model.LivestockQuotaPolicy, error) {
	var p model.LivestockQuotaPolicy
	if err := r.db.WithContext(ctx).Preload("Factors").
		Where("is_active = ?", true).
		Order("version DESC").First(&p).Error; err != nil {
		return nil, fmt.Errorf("no active livestock quota policy: %w", err)
	}
	return &p, nil
}

func (r *LivestockQuotaPolicyRepository) FindAll(ctx context.Context, offset, limit int) ([]model.LivestockQuotaPolicy, int64, error) {
	var policies []model.LivestockQuotaPolicy
	var total int64
	base := r.db.WithContext(ctx).Model(&model.LivestockQuotaPolicy{})
	base.Count(&total)
	if err := base.Preload("Factors").Offset(offset).Limit(limit).
		Order("name ASC, version DESC").Find(&policies).Error; err != nil {
		return nil, 0, err
	}
	return policies, total, nil
}

func (r *LivestockQuotaPolicyRepository) Update(ctx context.Context, p *model.LivestockQuotaPolicy) error {
	// Save would cascade-upsert the preloaded matrix; factor changes go through
	// ReplaceFactors so editing the policy cannot silently duplicate rows.
	return r.db.WithContext(ctx).Omit("Factors").Save(p).Error
}

func (r *LivestockQuotaPolicyRepository) ReplaceFactors(ctx context.Context, policyID uuid.UUID, factors []model.LivestockUGGFactor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("policy_id = ?", policyID).Delete(&model.LivestockUGGFactor{}).Error; err != nil {
			return err
		}
		if len(factors) == 0 {
			return nil
		}
		for i := range factors {
			factors[i].PolicyID = policyID
			if factors[i].ID == uuid.Nil {
				factors[i].ID = uuid.New()
			}
		}
		return tx.Create(&factors).Error
	})
}

func (r *LivestockQuotaPolicyRepository) SetActive(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.LivestockQuotaPolicy{}).
			Where("is_active = ?", true).
			Update("is_active", false).Error; err != nil {
			return err
		}
		res := tx.Model(&model.LivestockQuotaPolicy{}).Where("id = ?", id).Update("is_active", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("livestock quota policy %s not found", id)
		}
		return nil
	})
}
