package postgres

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// LivestockCreditPortfolioRepository stores the Odoo side of the livestock
// credit lines. Queries run with the SQL log lowered to warnings: candidates
// carry names and documents.
type LivestockCreditPortfolioRepository struct {
	db *gorm.DB
}

func NewLivestockCreditPortfolioRepository(db *gorm.DB) *LivestockCreditPortfolioRepository {
	return &LivestockCreditPortfolioRepository{db: db}
}

// odooColumns are the line's columns the portfolio writes; the rest of the
// row belongs to the activation.
var odooColumns = []string{
	"odoo_link_status", "odoo_partner_id", "odoo_candidates",
	"odoo_validation_status", "odoo_approved", "odoo_approved_amount", "odoo_used_amount",
	"odoo_available_amount", "odoo_overdue_days", "odoo_blocked", "odoo_can_buy",
	"odoo_block_reason", "odoo_promissory_note_date", "odoo_credit_updated_at",
	"odoo_snapshot_at", "odoo_last_sync_at", "odoo_last_sync_error", "odoo_last_sync_error_code",
}

func (r *LivestockCreditPortfolioRepository) SaveOdooState(ctx context.Context, line *model.LivestockCreditLine) error {
	if line.Odoo.LinkStatus == "" {
		line.Odoo.LinkStatus = model.OdooLinkPending
	}
	return r.quiet().WithContext(ctx).Model(&model.LivestockCreditLine{ID: line.ID}).
		Select(odooColumns).Updates(&model.LivestockCreditLine{Odoo: line.Odoo}).Error
}

func (r *LivestockCreditPortfolioRepository) ListLinesForSync(ctx context.Context, limit int) ([]model.LivestockCreditLine, error) {
	var lines []model.LivestockCreditLine
	err := r.quiet().WithContext(ctx).
		Where("status = ?", model.LivestockCreditLineActive).
		Where("odoo_link_status IS NULL OR odoo_link_status <> ?", model.OdooLinkAmbiguous).
		Order("odoo_last_sync_at ASC NULLS FIRST").Order("activated_at").
		Limit(limit).Find(&lines).Error
	return lines, err
}

func (r *LivestockCreditPortfolioRepository) UpsertInvoices(ctx context.Context, lineID uuid.UUID, invoices []model.LivestockCreditInvoice, complete bool) error {
	return r.quiet().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(invoices) > 0 {
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "credit_line_id"}, {Name: "odoo_invoice_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"number", "order_ref", "invoice_date", "due_date", "amount_total", "amount_residual",
					"payment_state", "installments", "synced_at", "updated_at",
				}),
			}).Create(&invoices).Error
			if err != nil {
				return err
			}
		}
		if !complete {
			return nil
		}
		ids := make([]int64, len(invoices))
		for i := range invoices {
			ids[i] = invoices[i].OdooInvoiceID
		}
		q := tx.Where("credit_line_id = ?", lineID)
		if len(ids) > 0 {
			q = q.Where("odoo_invoice_id NOT IN ?", ids)
		}
		return q.Delete(&model.LivestockCreditInvoice{}).Error
	})
}

func (r *LivestockCreditPortfolioRepository) ListInvoices(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditInvoice, error) {
	var invoices []model.LivestockCreditInvoice
	err := r.quiet().WithContext(ctx).Where("credit_line_id = ?", lineID).
		Order("due_date").Order("number").Find(&invoices).Error
	return invoices, err
}

func (r *LivestockCreditPortfolioRepository) CreatePayment(ctx context.Context, p *model.LivestockCreditPayment) error {
	return r.quiet().WithContext(ctx).Create(p).Error
}

func (r *LivestockCreditPortfolioRepository) FindActivePaymentByReference(ctx context.Context, reference string) (*model.LivestockCreditPayment, error) {
	var payments []model.LivestockCreditPayment
	err := r.quiet().WithContext(ctx).
		Where("reference = ? AND status <> ?", reference, model.PaymentFailed).
		Limit(1).Find(&payments).Error
	if err != nil || len(payments) == 0 {
		return nil, err
	}
	return &payments[0], nil
}

func (r *LivestockCreditPortfolioRepository) ListPayments(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditPayment, error) {
	var payments []model.LivestockCreditPayment
	err := r.quiet().WithContext(ctx).Where("credit_line_id = ?", lineID).
		Order("created_at DESC").Find(&payments).Error
	return payments, err
}

func (r *LivestockCreditPortfolioRepository) CreateBlockEvent(ctx context.Context, e *model.LivestockCreditBlockEvent) error {
	return r.quiet().WithContext(ctx).Create(e).Error
}

func (r *LivestockCreditPortfolioRepository) ListBlockEvents(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditBlockEvent, error) {
	var events []model.LivestockCreditBlockEvent
	err := r.quiet().WithContext(ctx).Where("credit_line_id = ?", lineID).
		Order("created_at DESC").Find(&events).Error
	return events, err
}

func (r *LivestockCreditPortfolioRepository) quiet() *gorm.DB {
	return r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Warn)})
}
