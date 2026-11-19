package service_test

import (
	"context"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// oneLoan returns the same loan for any id; nothing else may be called.
type oneLoan struct {
	port.LoanRepository
	loan *model.Loan
}

func (o oneLoan) FindByIDWithInstallments(context.Context, uuid.UUID) (*model.Loan, error) {
	return o.loan, nil
}

func TestPrestiasGenericFlowsRefuseALoanThatMirrorsAnOdooInvoice(t *testing.T) {
	loan := &model.Loan{ID: uuid.New(), Status: model.LoanActive, AmortizationType: model.AmortizationInterestIncluded,
		ExternalReference: "odoo-invoice:27506"}
	repo := oneLoan{loan: loan}
	ctx := context.Background()

	payments := service.NewPaymentService(nil, repo, nil, nil, nil, nil, decimal.Zero)
	_, err := payments.RecordPayment(ctx, uuid.New(), loan.ID, decimal.NewFromInt(100), model.PaymentTransfer, "R-1", nil)
	assert.ErrorIs(t, err, model.ErrPartnerManagedLoan)

	credits := service.NewCreditService(nil, repo, nil, nil, nil, nil, nil, nil)
	_, err = credits.PrepayLoan(ctx, uuid.New(), loan.ID, decimal.NewFromInt(100), model.PrepaymentStrategy("reduce_installment"))
	assert.ErrorIs(t, err, model.ErrPartnerManagedLoan)
	_, _, err = credits.CancelLoan(ctx, uuid.New(), loan.ID)
	assert.ErrorIs(t, err, model.ErrPartnerManagedLoan)
}
