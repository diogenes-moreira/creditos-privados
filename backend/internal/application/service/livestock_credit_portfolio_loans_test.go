package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loanBook plays the loans, the credit line and the two transactional
// repository methods.
type loanBook struct {
	creditLine   model.CreditLine
	loans        map[uuid.UUID]*model.Loan
	loanPayments []*model.Payment
}

// creditLines and loanRepo expose the book as the two repositories.
type creditLines struct {
	port.CreditLineRepository
	*loanBook
}

type loanRepo struct {
	port.LoanRepository
	*loanBook
}

func (b creditLines) FindByID(_ context.Context, id uuid.UUID) (*model.CreditLine, error) {
	if id != b.loanBook.creditLine.ID {
		return nil, errors.New("credit line not found")
	}
	cl := b.loanBook.creditLine
	return &cl, nil
}

func (b loanRepo) FindByIDWithInstallments(_ context.Context, id uuid.UUID) (*model.Loan, error) {
	l, ok := b.loanBook.loans[id]
	if !ok {
		return nil, errors.New("loan not found")
	}
	copy := *l
	copy.Installments = append([]model.Installment(nil), l.Installments...)
	return &copy, nil
}

// loanStore adds the transactional methods to the portfolio store.
type loanStore struct {
	*portfolioStore
	book *loanBook
}

func (s *loanStore) OpenInvoiceLoan(_ context.Context, invoiceID uuid.UUID, loan *model.Loan) error {
	for lineID, invs := range s.invoices {
		for odooID, inv := range invs {
			if inv.ID == invoiceID {
				id := loan.ID
				inv.LoanID = &id
				s.invoices[lineID][odooID] = inv
				s.book.loans[loan.ID] = loan
				s.book.creditLine.UsedAmount = s.book.creditLine.UsedAmount.Add(loan.InvoiceTotal())
				return nil
			}
		}
	}
	return errors.New("invoice not found")
}

func (s *loanStore) RecordLoanPayment(_ context.Context, loan *model.Loan, p *model.Payment, released decimal.Decimal, payment *model.LivestockCreditPayment) error {
	s.book.loans[loan.ID] = loan
	s.book.loanPayments = append(s.book.loanPayments, p)
	s.book.creditLine.UsedAmount = s.book.creditLine.UsedAmount.Sub(released)
	return s.portfolioStore.CreatePayment(context.Background(), payment)
}

// loanPortfolio is a line enrolled in Prestia (client and credit line at
// 26.01 % TEA), synced from Odoo: its two quota invoices have loans.
func loanPortfolio(t *testing.T) (*service.LivestockCreditPortfolioService, *fakeOdoo, *loanStore, uuid.UUID) {
	t.Helper()
	odoo := newFakeOdoo()
	base := newPortfolioStore()
	book := &loanBook{loans: map[uuid.UUID]*model.Loan{}}
	book.creditLine = model.CreditLine{ID: uuid.New(), MaxAmount: dec("5000000"), UsedAmount: decimal.Zero,
		InterestRate: dec("23.3433"), MaxInstallments: 3, Status: model.CreditLineApproved}
	store := &loanStore{portfolioStore: base, book: book}
	lineID := base.addLine("1000000103")
	line := base.lines[lineID]
	clientID := uuid.New()
	line.ClientID, line.CreditLineID = &clientID, &book.creditLine.ID
	base.lines[lineID] = line
	s := service.NewLivestockCreditPortfolioService(base, store, odoo).WithLoans(creditLines{loanBook: book}, loanRepo{loanBook: book}).
		WithClock(func() time.Time { return portfolioToday })
	_, err := s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	odoo.calls = map[string]int{}
	return s, odoo, store, lineID
}

func loanOf(t *testing.T, store *loanStore, lineID uuid.UUID, odooInvoiceID int64) *model.Loan {
	t.Helper()
	inv := store.invoices[lineID][odooInvoiceID]
	require.NotNil(t, inv.LoanID, "invoice %d has a loan", odooInvoiceID)
	return store.book.loans[*inv.LoanID]
}

func TestEveryQuotaInvoiceOpensAnInterestIncludedLoan(t *testing.T) {
	s, odoo, store, lineID := loanPortfolio(t)

	big := loanOf(t, store, lineID, 27506)
	assert.Equal(t, model.AmortizationInterestIncluded, big.AmortizationType)
	assert.Equal(t, "odoo-invoice:27506", big.ExternalReference)
	assert.True(t, big.PartnerManaged())
	require.Len(t, big.Installments, 3)
	assert.Equal(t, "825260.80", big.Installments[0].TotalAmount.StringFixed(2), "Odoo's installment, interest included")
	assert.Equal(t, "2026-12-12", big.Installments[0].DueDate.Format("2006-01-02"), "Odoo's date")
	assert.Equal(t, "2382728.70", big.Principal.StringFixed(2), "the capital worked out of the price")
	assert.Equal(t, "794163.48", big.Installments[0].CapitalAmount.StringFixed(2))
	assert.Equal(t, "31097.32", big.Installments[0].InterestAmount.StringFixed(2))
	assert.Equal(t, "2576030.00", store.book.creditLine.UsedAmount.StringFixed(2), "the quota is used by the invoices' totals")
	assert.Zero(t, odoo.calls["approved"], "Odoo's approved quota already matches the credit line")

	// Syncing again opens nothing new.
	_, err := s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	assert.Len(t, store.book.loans, 2)
	assert.Equal(t, "2576030.00", store.book.creditLine.UsedAmount.StringFixed(2))
}

func TestOdoosApprovedQuotaFollowsTheCreditLine(t *testing.T) {
	s, odoo, store, lineID := loanPortfolio(t)
	// The quota changes in Prestia (the credit line's max amount).
	store.book.creditLine.MaxAmount = dec("6000000")

	p, err := s.Sync(context.Background(), lineID)

	require.NoError(t, err)
	require.Len(t, odoo.approved, 1)
	assert.Equal(t, "6000000.00", odoo.approved[0].StringFixed(2))
	assert.Equal(t, "6000000.00", p.Line.Odoo.ApprovedAmount.StringFixed(2), "the snapshot is Odoo's answer")
	assert.Equal(t, "3423970.00", p.Line.Odoo.AvailableAmount.StringFixed(2))

	_, err = s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	assert.Len(t, odoo.approved, 1, "aligned: nothing more to send")
}

func TestAPaymentGoesWholeToOdooAndIsSplitInPrestia(t *testing.T) {
	s, odoo, store, lineID := loanPortfolio(t)
	// FE12263: one installment of 100,000 (interest included).
	out, err := s.RegisterPayment(context.Background(), lineID, payReq(27507, "100000", "DAVIVIENDA", "PRESTIA-1"), staff)

	require.NoError(t, err)
	assert.Equal(t, model.PaymentRegistered, out.Payment.Status)
	require.Len(t, odoo.payments, 1)
	assert.Equal(t, "100000.00", odoo.payments[0].Amount.StringFixed(2), "Odoo gets the whole payment")
	assert.Equal(t, "98091.84", out.Payment.CapitalAmount.StringFixed(2))
	assert.Equal(t, "1908.16", out.Payment.InterestAmount.StringFixed(2))
	assert.NotNil(t, out.Payment.InterestReportedAt, "the interest is in Odoo's invoice")
	assert.Equal(t, model.LoanCompleted, loanOf(t, store, lineID, 27507).Status)
	require.Len(t, store.book.loanPayments, 1)
	assert.Equal(t, *out.Payment.LoanPaymentID, store.book.loanPayments[0].ID)
	assert.Equal(t, "2476030.00", store.book.creditLine.UsedAmount.StringFixed(2), "the quota is freed by the whole payment")
}

func TestAPartialPaymentGoesToTheOldestInstallmentInterestFirst(t *testing.T) {
	s, _, store, lineID := loanPortfolio(t)

	out, err := s.RegisterPayment(context.Background(), lineID, payReq(27506, "500000", "BANCOLOMBIA", "PRESTIA-2"), staff)

	require.NoError(t, err)
	assert.Equal(t, "31097.32", out.Payment.InterestAmount.StringFixed(2))
	assert.Equal(t, "468902.68", out.Payment.CapitalAmount.StringFixed(2))
	assert.Equal(t, model.InstallmentPartial, loanOf(t, store, lineID, 27506).Installments[0].Status)
}

func TestAPaymentOverTheBalanceOrWithOdooDownChangesNothing(t *testing.T) {
	s, odoo, store, lineID := loanPortfolio(t)

	_, err := s.RegisterPayment(context.Background(), lineID, payReq(27507, "100000.01", "DAVIVIENDA", "PRESTIA-3"), staff)
	assert.Equal(t, "amount_exceeds_residual", rejection(t, err).Code)
	assert.Empty(t, odoo.payments)

	odoo.pay = func(int64, model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
		return nil, &port.OdooError{Status: 503, Code: "UNAVAILABLE", Message: "down"}
	}
	_, err = s.RegisterPayment(context.Background(), lineID, payReq(27507, "50000", "DAVIVIENDA", "PRESTIA-4"), staff)
	assert.Equal(t, model.PortfolioUnavailable, rejection(t, err).Kind)
	assert.Empty(t, store.payments)
	assert.Empty(t, store.book.loanPayments)
	assert.Equal(t, model.InstallmentPending, loanOf(t, store, lineID, 27507).Installments[0].Status)
}
