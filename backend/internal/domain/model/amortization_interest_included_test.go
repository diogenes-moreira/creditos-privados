package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterestIncludedIsTheFrenchInstallmentWithEvenCapitalAndInterest(t *testing.T) {
	d := decimal.RequireFromString
	start := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

	s := CalculateInterestIncludedAmortization(d("101.04"), d(cuppoganRate), 3, start, decimal.Zero)

	require.Len(t, s.Installments, 3)
	for _, i := range s.Installments {
		assert.Equal(t, "35.00", i.Total.StringFixed(2))
		assert.Equal(t, "33.68", i.Capital.StringFixed(2))
		assert.Equal(t, "1.32", i.Interest.StringFixed(2))
	}
	assert.Equal(t, "2026-12-20", s.Installments[0].DueDate.Format(time.DateOnly))
	assert.Equal(t, "3.96", s.TotalInterest.StringFixed(2))
	assert.Equal(t, "105.00", s.TotalPayment.StringFixed(2))
	french := CalculateFrenchAmortization(d("101.04"), d(cuppoganRate), 3, start, decimal.Zero)
	assert.Equal(t, french.Installments[0].Total, s.Installments[0].Total, "same installment as French")
	assert.Equal(t, "1.97", french.Installments[0].Interest.StringFixed(2), "French puts more interest first")

	withIVA := CalculateInterestIncludedAmortization(d("101.04"), d(cuppoganRate), 3, start, d("21"))
	assert.Equal(t, "0.28", withIVA.Installments[0].IVA.StringFixed(2), "IVA on the interest, like the other systems")
}

func TestAPrestiaLoanCanUseTheInterestIncludedSystem(t *testing.T) {
	d := decimal.RequireFromString
	loan, err := NewLoan(uuid.New(), uuid.New(), d("101.04"), d(cuppoganRate), 3, AmortizationInterestIncluded)
	require.NoError(t, err)
	assert.False(t, loan.PartnerManaged(), "Prestia's own loan: its generic flows apply")
	require.NoError(t, loan.RequestApproval())
	require.NoError(t, loan.Approve(uuid.New()))

	installments, err := loan.Disburse(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), decimal.Zero)

	require.NoError(t, err)
	require.Len(t, installments, 3)
	assert.Equal(t, "35.00", installments[2].TotalAmount.StringFixed(2))

	// A prepayment recomputes the rest with the same system.
	loan.Installments[0].Status = InstallmentPaid
	loan.RecalculateRemainingInstallments(d("50"), d(cuppoganRate), decimal.Zero)
	assert.Equal(t, "25.00", loan.Installments[1].CapitalAmount.StringFixed(2))
	assert.Equal(t, loan.Installments[1].TotalAmount, loan.Installments[2].TotalAmount)
}
