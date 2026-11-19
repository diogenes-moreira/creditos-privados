package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An Odoo quota invoice of $1,000,000 with the "Cuppogan" term: 3 installments
// at 30, 60 and 90 days. 23.3433 is 26.01 % TEA.
func quotaInvoiceLoan(t *testing.T) *Loan {
	t.Helper()
	d := decimal.RequireFromString
	inv := LivestockCreditInvoice{Number: "FE100", InvoiceDate: "2026-11-20", DueDate: "2027-02-18",
		AmountTotal: d("1000000"), Installments: []LivestockCreditInstallment{
			{Number: 1, DueDate: "2026-12-20", Amount: d("333300"), Residual: d("333300")},
			{Number: 2, DueDate: "2027-01-19", Amount: d("333300"), Residual: d("333300")},
			{Number: 3, DueDate: "2027-02-18", Amount: d("333400"), Residual: d("333400")},
		}}
	loan, err := NewLivestockInvoiceLoan(uuid.New(), uuid.New(), inv, d("23.3433"))
	require.NoError(t, err)
	return loan
}

func day(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func amounts(loan *Loan, f func(Installment) decimal.Decimal) []string {
	var out []string
	for _, i := range loan.Installments {
		out = append(out, f(i).StringFixed(2))
	}
	return out
}

func TestAQuotaInvoiceLoanHasOdoosCapitalAndDatesPlusInterest(t *testing.T) {
	loan := quotaInvoiceLoan(t)

	assert.Equal(t, LoanActive, loan.Status)
	assert.Equal(t, AmortizationGerman, loan.AmortizationType)
	assert.Equal(t, "1000000.00", loan.Principal.StringFixed(2))
	assert.Equal(t, "2026-11-20", loan.DisbursedAt.Format(time.DateOnly))
	assert.Equal(t, []string{"333300.00", "333300.00", "333400.00"}, amounts(loan, func(i Installment) decimal.Decimal { return i.CapitalAmount }))
	assert.Equal(t, []string{"19452.75", "12969.15", "6485.55"}, amounts(loan, func(i Installment) decimal.Decimal { return i.InterestAmount }))
	assert.Equal(t, "2027-01-19", loan.Installments[1].DueDate.Format(time.DateOnly))
	for _, i := range loan.Installments {
		assert.True(t, i.IVAAmount.IsZero())
		assert.Equal(t, i.TotalAmount, i.RemainingAmount)
	}
}

func TestPayingADueInstallmentGoesToInterestFirst(t *testing.T) {
	loan := quotaInvoiceLoan(t)

	partial, err := loan.ApplyLivestockPayment(decimal.NewFromInt(20000), day("2026-12-20"))
	require.NoError(t, err)
	assert.Equal(t, "19452.75", partial.Interest.StringFixed(2))
	assert.Equal(t, "547.25", partial.Capital.StringFixed(2))
	assert.Equal(t, InstallmentPartial, loan.Installments[0].Status)

	rest, err := loan.ApplyLivestockPayment(decimal.RequireFromString("332752.75"), day("2026-12-21"))
	require.NoError(t, err)
	assert.True(t, rest.Interest.IsZero())
	assert.Equal(t, "332752.75", rest.Capital.StringFixed(2))
	assert.Equal(t, InstallmentPaid, loan.Installments[0].Status)
	assert.Equal(t, "12969.15", loan.Installments[1].InterestAmount.StringFixed(2), "no prepayment: interest unchanged")
}

func TestAPrepaymentGoesToCapitalAndLowersTheInterestNotYetDue(t *testing.T) {
	loan := quotaInvoiceLoan(t)

	split, err := loan.ApplyLivestockPayment(decimal.NewFromInt(500000), day("2026-11-30"))

	require.NoError(t, err)
	assert.True(t, split.Interest.IsZero(), "nothing was due yet")
	assert.Equal(t, "500000.00", split.Capital.StringFixed(2))
	assert.Equal(t, []string{"333300.00", "166700.00", "0.00"}, amounts(loan, func(i Installment) decimal.Decimal { return i.PaidCapital }))
	assert.Equal(t, []string{"9726.38", "9726.38", "6485.55"}, amounts(loan, func(i Installment) decimal.Decimal { return i.InterestAmount }))
	assert.Equal(t, InstallmentPartial, loan.Installments[0].Status, "its interest is still owed at its due date")
	assert.Equal(t, "500000.00", loan.OutstandingCapital().StringFixed(2))
}

func TestPayingEverythingCompletesTheLoanAndNoMore(t *testing.T) {
	loan := quotaInvoiceLoan(t)
	on := day("2027-03-01")
	payoff := loan.LivestockPayoff(on)
	assert.Equal(t, "1038907.45", payoff.StringFixed(2))

	_, err := loan.ApplyLivestockPayment(payoff.Add(decimal.NewFromInt(1)), on)
	assert.Error(t, err)

	split, err := loan.ApplyLivestockPayment(payoff, on)
	require.NoError(t, err)
	assert.Equal(t, "1000000.00", split.Capital.StringFixed(2))
	assert.Equal(t, "38907.45", split.Interest.StringFixed(2))
	assert.Equal(t, LoanCompleted, loan.Status)
}

func TestAnInvoiceWithoutInstallmentsIsOneInstallmentAtItsDueDate(t *testing.T) {
	inv := LivestockCreditInvoice{Number: "FE101", InvoiceDate: "2026-11-20", DueDate: "2026-12-20", AmountTotal: decimal.NewFromInt(300000)}
	loan, err := NewLivestockInvoiceLoan(uuid.New(), uuid.New(), inv, decimal.RequireFromString("23.3433"))
	require.NoError(t, err)
	require.Len(t, loan.Installments, 1)
	assert.Equal(t, "300000.00", loan.Installments[0].CapitalAmount.StringFixed(2))
}
