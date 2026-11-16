package postgres

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type LivestockCreditLineRepository struct {
	db *gorm.DB
}

func NewLivestockCreditLineRepository(db *gorm.DB) *LivestockCreditLineRepository {
	return &LivestockCreditLineRepository{db: db}
}

// Create stores the line and its promissory note in one transaction. The SQL
// log is lowered to warnings for it: at the default level GORM would print the
// rancher's personal data and the PDF.
func (r *LivestockCreditLineRepository) Create(ctx context.Context, line *model.LivestockCreditLine, doc *model.LivestockCreditLineDocument) error {
	quiet := r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Warn)})
	return quiet.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Assessment").Create(line).Error; err != nil {
			return err
		}
		return tx.Create(doc).Error
	})
}

func (r *LivestockCreditLineRepository) FindByPromissoryNote(ctx context.Context, number string) (*model.LivestockCreditLine, error) {
	return r.findOne(ctx, "promissory_note_number = ?", number)
}

func (r *LivestockCreditLineRepository) FindByPartnerReference(ctx context.Context, partner, reference string) (*model.LivestockCreditLine, error) {
	return r.findOne(ctx, "partner = ? AND partner_reference = ?", partner, reference)
}

func (r *LivestockCreditLineRepository) FindByAssessment(ctx context.Context, assessmentID uuid.UUID) (*model.LivestockCreditLine, error) {
	return r.findOne(ctx, "assessment_id = ?", assessmentID)
}

func (r *LivestockCreditLineRepository) findOne(ctx context.Context, query string, args ...any) (*model.LivestockCreditLine, error) {
	// Find, not First: no match is an answer here, not an error to log.
	var lines []model.LivestockCreditLine
	if err := r.db.WithContext(ctx).Where(query, args...).Limit(1).Find(&lines).Error; err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return &lines[0], nil
}
