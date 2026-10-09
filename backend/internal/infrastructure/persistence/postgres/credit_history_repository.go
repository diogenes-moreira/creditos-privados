package postgres

import (
	"context"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type CreditHistoryRepository struct {
	db *gorm.DB
}

func NewCreditHistoryRepository(db *gorm.DB) *CreditHistoryRepository {
	return &CreditHistoryRepository{db: db}
}

// installmentRow is the projection the behaviour scoring needs. Delay is
// computed in Go rather than in the query so the arithmetic stays portable and
// testable; per-client volumes are small enough that this is cheap.
type installmentRow struct {
	DueDate         time.Time
	PaidAt          *time.Time
	Status          string
	RemainingAmount decimal.Decimal
}

// ClientCreditHistory projects a client's loans and installments into the
// repayment-behaviour summary the risk analysis consumes.
func (r *CreditHistoryRepository) ClientCreditHistory(ctx context.Context, clientID uuid.UUID) (*model.RiskHistory, error) {
	history := &model.RiskHistory{
		CurrentOverdueAmount: decimal.Zero,
		OutstandingBalance:   decimal.Zero,
	}

	var loansTotal, loansActive int64
	if err := r.db.WithContext(ctx).Model(&model.Loan{}).
		Where("client_id = ?", clientID).Count(&loansTotal).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&model.Loan{}).
		Where("client_id = ? AND status = ?", clientID, model.LoanActive).
		Count(&loansActive).Error; err != nil {
		return nil, err
	}
	history.LoansTotal = int(loansTotal)
	history.LoansActive = int(loansActive)

	now := time.Now()
	var rows []installmentRow
	if err := r.db.WithContext(ctx).Model(&model.Installment{}).
		Select("installments.due_date, installments.paid_at, installments.status, installments.remaining_amount").
		Joins("JOIN loans ON loans.id = installments.loan_id").
		Where("loans.client_id = ? AND loans.deleted_at IS NULL", clientID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		switch {
		case row.PaidAt != nil:
			// A settled installment counts toward the track record either way.
			history.InstallmentsDue++
			if row.PaidAt.After(row.DueDate) {
				history.InstallmentsLate++
				if days := daysBetween(row.DueDate, *row.PaidAt); days > history.MaxDaysLate {
					history.MaxDaysLate = days
				}
			} else {
				history.InstallmentsPaidOnTime++
			}
		case row.DueDate.Before(now):
			// Past due and still unpaid: counts as due, and as current arrears.
			history.InstallmentsDue++
			history.InstallmentsLate++
			if days := daysBetween(row.DueDate, now); days > history.MaxDaysLate {
				history.MaxDaysLate = days
			}
			history.CurrentOverdueAmount = history.CurrentOverdueAmount.Add(row.RemainingAmount)
		}
		// Installments not yet due are outside the track record entirely.
		if row.Status != string(model.InstallmentPaid) {
			history.OutstandingBalance = history.OutstandingBalance.Add(row.RemainingAmount)
		}
	}

	history.CurrentOverdueAmount = history.CurrentOverdueAmount.Round(2)
	history.OutstandingBalance = history.OutstandingBalance.Round(2)
	return history, nil
}

func daysBetween(from, to time.Time) int {
	if to.Before(from) {
		return 0
	}
	return int(to.Sub(from).Hours() / 24)
}
