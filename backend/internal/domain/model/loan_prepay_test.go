package model_test

import (
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDisbursedLoan returns an active loan with a generated installment schedule.
// interestRate is an annual rate expressed as a fraction (0.24 = 24% annual),
// matching the convention used by the amortization calculators.
func newDisbursedLoan(t *testing.T, amortType model.AmortizationType, numInstallments int) *model.Loan {
	t.Helper()
	loan, err := model.NewLoan(
		uuid.New(),
		uuid.New(),
		decimal.NewFromInt(120000),
		decimal.NewFromInt(24),
		numInstallments,
		amortType,
	)
	require.NoError(t, err)

	require.NoError(t, loan.RequestApproval())
	require.NoError(t, loan.Approve(uuid.New()))
	// Start a year back so every installment in a 12-month schedule is past due.
	start := time.Now().AddDate(-1, -1, 0)
	_, err = loan.Disburse(start, decimal.NewFromInt(21))
	require.NoError(t, err)
	require.Len(t, loan.Installments, numInstallments)
	return loan
}

func TestLoanOverdueInstallments(t *testing.T) {
	loan := newDisbursedLoan(t, model.AmortizationFrench, 12)

	// All 12 due dates are in the past and every installment is still pending.
	assert.Len(t, loan.OverdueInstallments(), 12)

	// Paying one installment removes it from the overdue set.
	loan.Installments[0].Status = model.InstallmentPaid
	assert.Len(t, loan.OverdueInstallments(), 11)

	// A future due date is not overdue.
	loan.Installments[1].DueDate = time.Now().AddDate(0, 1, 0)
	assert.Len(t, loan.OverdueInstallments(), 10)

	// Only pending installments count: partial/overdue statuses are excluded.
	loan.Installments[2].Status = model.InstallmentPartial
	assert.Len(t, loan.OverdueInstallments(), 9)
}

func TestLoanOutstandingPrincipal(t *testing.T) {
	loan := newDisbursedLoan(t, model.AmortizationFrench, 12)

	// With nothing paid, outstanding principal is the full capital, which for a
	// fully amortizing schedule equals the original principal.
	assert.True(t, loan.OutstandingPrincipal().Equal(loan.Principal),
		"expected %s, got %s", loan.Principal, loan.OutstandingPrincipal())

	// Paying installments excludes their capital from the outstanding balance.
	paidCapital := loan.Installments[0].CapitalAmount.Add(loan.Installments[1].CapitalAmount)
	loan.Installments[0].Status = model.InstallmentPaid
	loan.Installments[1].Status = model.InstallmentPaid

	assert.True(t, loan.OutstandingPrincipal().Equal(loan.Principal.Sub(paidCapital)))

	// Fully paid loan owes nothing.
	for i := range loan.Installments {
		loan.Installments[i].Status = model.InstallmentPaid
	}
	assert.True(t, loan.OutstandingPrincipal().IsZero())
}

func TestLoanRecalculateRemainingInstallments(t *testing.T) {
	rate := decimal.NewFromInt(24)
	iva := decimal.NewFromInt(21)

	t.Run("reduces installment amounts and keeps the term", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		loan.Installments[0].Status = model.InstallmentPaid
		originalTotal := loan.Installments[1].TotalAmount
		pendingBefore := len(loan.Installments) - 1

		// Prepay: outstanding drops to half of the remaining capital.
		newOutstanding := loan.OutstandingPrincipal().Div(decimal.NewFromInt(2)).Round(2)
		loan.RecalculateRemainingInstallments(newOutstanding, rate, iva)

		// Term is unchanged; the paid installment is untouched.
		assert.Len(t, loan.Installments, 12)
		assert.Equal(t, model.InstallmentPaid, loan.Installments[0].Status)

		// Every pending installment got cheaper and was reset to unpaid.
		pendingAfter := 0
		for _, inst := range loan.Installments[1:] {
			pendingAfter++
			assert.True(t, inst.TotalAmount.LessThan(originalTotal),
				"installment %d: %s should be < %s", inst.Number, inst.TotalAmount, originalTotal)
			assert.True(t, inst.PaidAmount.IsZero())
			assert.True(t, inst.RemainingAmount.Equal(inst.TotalAmount))
		}
		assert.Equal(t, pendingBefore, pendingAfter)

		// Capital across pending installments re-amortizes the new outstanding balance.
		assert.InDelta(t, newOutstanding.InexactFloat64(),
			loan.OutstandingPrincipal().InexactFloat64(), 1.0)
	})

	t.Run("no-ops when nothing is pending", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		for i := range loan.Installments {
			loan.Installments[i].Status = model.InstallmentPaid
		}
		before := loan.Installments[0].TotalAmount

		loan.RecalculateRemainingInstallments(decimal.NewFromInt(50000), rate, iva)

		assert.True(t, loan.Installments[0].TotalAmount.Equal(before))
	})

	t.Run("no-ops on zero outstanding principal", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		before := loan.Installments[0].TotalAmount

		loan.RecalculateRemainingInstallments(decimal.Zero, rate, iva)

		assert.True(t, loan.Installments[0].TotalAmount.Equal(before))
	})

	t.Run("works for german amortization", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationGerman, 12)
		newOutstanding := loan.OutstandingPrincipal().Div(decimal.NewFromInt(4)).Round(2)

		loan.RecalculateRemainingInstallments(newOutstanding, rate, iva)

		assert.Len(t, loan.Installments, 12)
		// German amortization repays a constant capital slice each period.
		first := loan.Installments[0].CapitalAmount
		for _, inst := range loan.Installments[1:] {
			assert.InDelta(t, first.InexactFloat64(), inst.CapitalAmount.InexactFloat64(), 0.05)
		}
	})
}

func TestLoanRecalculateReducingTerm(t *testing.T) {
	rate := decimal.NewFromInt(24)
	iva := decimal.NewFromInt(21)

	t.Run("drops installments and returns their ids", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		// Prepay down to a quarter of the balance: the term must shrink.
		newOutstanding := loan.OutstandingPrincipal().Div(decimal.NewFromInt(4)).Round(2)

		removed := loan.RecalculateReducingTerm(newOutstanding, rate, iva)

		require.NotEmpty(t, removed, "expected the term to be reduced")
		assert.Len(t, loan.Installments, 12-len(removed))

		// Removed ids must no longer be part of the loan.
		remaining := make(map[uuid.UUID]bool, len(loan.Installments))
		for _, inst := range loan.Installments {
			remaining[inst.ID] = true
		}
		for _, id := range removed {
			assert.False(t, remaining[id], "installment %s should have been removed", id)
		}

		// The surviving schedule amortizes the new outstanding balance.
		assert.InDelta(t, newOutstanding.InexactFloat64(),
			loan.OutstandingPrincipal().InexactFloat64(), 1.0)
	})

	t.Run("keeps paid installments and only trims pending ones", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		paidID := loan.Installments[0].ID
		loan.Installments[0].Status = model.InstallmentPaid

		newOutstanding := loan.OutstandingPrincipal().Div(decimal.NewFromInt(4)).Round(2)
		removed := loan.RecalculateReducingTerm(newOutstanding, rate, iva)
		require.NotEmpty(t, removed)

		for _, id := range removed {
			assert.NotEqual(t, paidID, id, "a paid installment must never be removed")
		}
		assert.Equal(t, model.InstallmentPaid, loan.Installments[0].Status)
	})

	t.Run("returns nil when no reduction is possible", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)

		// Outstanding unchanged: the term cannot shrink.
		assert.Nil(t, loan.RecalculateReducingTerm(loan.OutstandingPrincipal(), rate, iva))
		assert.Len(t, loan.Installments, 12)

		// Zero outstanding is a no-op.
		assert.Nil(t, loan.RecalculateReducingTerm(decimal.Zero, rate, iva))
		assert.Len(t, loan.Installments, 12)
	})

	t.Run("returns nil when nothing is pending", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		for i := range loan.Installments {
			loan.Installments[i].Status = model.InstallmentPaid
		}

		assert.Nil(t, loan.RecalculateReducingTerm(decimal.NewFromInt(1000), rate, iva))
	})

	t.Run("reduces term for german amortization", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationGerman, 12)
		// German capital slices are constant, so a quarter of the balance
		// should leave roughly a quarter of the installments.
		newOutstanding := loan.OutstandingPrincipal().Div(decimal.NewFromInt(4)).Round(2)

		removed := loan.RecalculateReducingTerm(newOutstanding, rate, iva)

		require.NotEmpty(t, removed)
		assert.Len(t, loan.Installments, 3)
	})
}

func TestLoanCancellationSettlement(t *testing.T) {
	iva := decimal.NewFromInt(21)

	t.Run("totals pending capital plus accrued interest and its iva", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		loan.Installments[0].Status = model.InstallmentPaid
		// Disburse() stamps DisbursedAt as "now", so wind it back to let interest accrue.
		disbursed := time.Now().AddDate(0, -6, 0)
		loan.DisbursedAt = &disbursed

		capital, interest, ivaAmount, total := loan.CancellationSettlement(iva)

		// Pending capital excludes the paid installment.
		assert.True(t, capital.Equal(loan.OutstandingPrincipal()))
		assert.True(t, interest.IsPositive(), "interest should accrue since disbursement")

		// IVA is charged on the accrued interest at the given rate.
		expectedIVA := interest.Mul(iva).Div(decimal.NewFromInt(100)).Round(2)
		assert.True(t, ivaAmount.Equal(expectedIVA),
			"expected iva %s, got %s", expectedIVA, ivaAmount)

		// Total is the sum of the three components.
		assert.True(t, total.Equal(capital.Add(interest).Add(ivaAmount)),
			"expected total %s, got %s", capital.Add(interest).Add(ivaAmount), total)
	})

	t.Run("no interest accrues without a disbursement date", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		loan.DisbursedAt = nil

		capital, interest, ivaAmount, total := loan.CancellationSettlement(iva)

		assert.True(t, capital.IsPositive())
		assert.True(t, interest.IsZero())
		assert.True(t, ivaAmount.IsZero())
		assert.True(t, total.Equal(capital))
	})

	t.Run("fully paid loan settles at zero", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		for i := range loan.Installments {
			loan.Installments[i].Status = model.InstallmentPaid
		}

		capital, interest, ivaAmount, total := loan.CancellationSettlement(iva)

		assert.True(t, capital.IsZero())
		assert.True(t, interest.IsZero())
		assert.True(t, ivaAmount.IsZero())
		assert.True(t, total.IsZero())
	})

	t.Run("zero iva rate charges interest only", func(t *testing.T) {
		loan := newDisbursedLoan(t, model.AmortizationFrench, 12)
		disbursed := time.Now().AddDate(0, -6, 0)
		loan.DisbursedAt = &disbursed

		capital, interest, ivaAmount, total := loan.CancellationSettlement(decimal.Zero)

		assert.True(t, ivaAmount.IsZero())
		assert.True(t, total.Equal(capital.Add(interest)))
	})
}
