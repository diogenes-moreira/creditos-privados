package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 23.3433 nominal annual = 1.945271 % a month = 26.01 % TEA.
const cuppoganRate = "23.3433"

func invoiceLoan(t *testing.T, invoiceDate string, amounts ...string) *Loan {
	t.Helper()
	inv := LivestockCreditInvoice{Number: "FE100", InvoiceDate: invoiceDate}
	start, _ := time.Parse(time.DateOnly, invoiceDate)
	total := decimal.Zero
	for i, a := range amounts {
		d := decimal.RequireFromString(a)
		total = total.Add(d)
		inv.Installments = append(inv.Installments, LivestockCreditInstallment{Number: i + 1,
			DueDate: start.AddDate(0, 0, 30*(i+1)).Format(time.DateOnly), Amount: d, Residual: d})
	}
	inv.AmountTotal = total
	loan, err := NewLivestockInvoiceLoan(uuid.New(), uuid.New(), inv, decimal.RequireFromString(cuppoganRate))
	require.NoError(t, err)
	return loan
}

func day(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func column(loan *Loan, f func(Installment) decimal.Decimal) []string {
	var out []string
	for _, i := range loan.Installments {
		out = append(out, f(i).StringFixed(2))
	}
	return out
}

func capitalOf(i Installment) decimal.Decimal  { return i.CapitalAmount }
func interestOf(i Installment) decimal.Decimal { return i.InterestAmount }

func TestTheInterestIsWorkedOutOfTheInvoicePrice(t *testing.T) {
	// The worked example: a $105 invoice in 3 installments of $35.
	loan := invoiceLoan(t, "2026-11-20", "35", "35", "35")

	assert.Equal(t, AmortizationInterestIncluded, loan.AmortizationType)
	assert.True(t, loan.PartnerManaged())
	assert.Equal(t, LoanActive, loan.Status)
	assert.Equal(t, "101.04", loan.Principal.StringFixed(2), "capital: the present value of the installments")
	assert.Equal(t, []string{"33.68", "33.68", "33.68"}, column(loan, capitalOf))
	assert.Equal(t, []string{"1.32", "1.32", "1.32"}, column(loan, interestOf), "the same interest in each installment")
	assert.Equal(t, "105.00", loan.InvoiceTotal().StringFixed(2))
	assert.Equal(t, "2026-12-20", loan.Installments[0].DueDate.Format(time.DateOnly), "Odoo's dates")
}

func TestOdoosThirdsKeepTheirAmountsAndTheCapitalAddsUp(t *testing.T) {
	loan := invoiceLoan(t, "2026-11-20", "333300", "333300", "333400")

	assert.Equal(t, "962318.19", loan.Principal.StringFixed(2))
	assert.Equal(t, []string{"320740.65", "320740.65", "320836.89"}, column(loan, capitalOf))
	assert.Equal(t, []string{"12559.35", "12559.35", "12563.11"}, column(loan, interestOf))
	assert.Equal(t, []string{"333300.00", "333300.00", "333400.00"}, column(loan, func(i Installment) decimal.Decimal { return i.TotalAmount }))
	for _, i := range loan.Installments {
		assert.True(t, i.IVAAmount.IsZero())
	}
}

func TestAPaymentGoesToTheOldestInstallmentInterestFirst(t *testing.T) {
	loan := invoiceLoan(t, "2026-11-20", "35", "35", "35")

	split, err := loan.ApplyLivestockPayment(decimal.RequireFromString("10"), day("2026-12-20"))
	require.NoError(t, err)
	assert.Equal(t, "1.32", split.Interest.StringFixed(2))
	assert.Equal(t, "8.68", split.Capital.StringFixed(2))
	assert.Equal(t, InstallmentPartial, loan.Installments[0].Status)

	split, err = loan.ApplyLivestockPayment(decimal.RequireFromString("25"), day("2026-12-21"))
	require.NoError(t, err)
	assert.True(t, split.Interest.IsZero())
	assert.Equal(t, InstallmentPaid, loan.Installments[0].Status)
}

func TestAnEarlyPaymentPaysWholeInstallmentsWithoutDiscount(t *testing.T) {
	loan := invoiceLoan(t, "2026-11-20", "35", "35", "35")
	on := day("2026-11-25")
	assert.Equal(t, "105.00", loan.LivestockPayoff(on).StringFixed(2), "the interest is in the price")
	assert.True(t, loan.LivestockDue(on).IsZero())

	_, err := loan.ApplyLivestockPayment(decimal.RequireFromString("105.01"), on)
	assert.Error(t, err)

	split, err := loan.ApplyLivestockPayment(decimal.RequireFromString("105"), on)
	require.NoError(t, err)
	assert.Equal(t, "101.04", split.Capital.StringFixed(2))
	assert.Equal(t, "3.96", split.Interest.StringFixed(2))
	assert.Equal(t, LoanCompleted, loan.Status)
}

func TestAnInvoiceWithoutInstallmentsIsOneInstallmentAtItsDueDate(t *testing.T) {
	inv := LivestockCreditInvoice{Number: "FE101", InvoiceDate: "2026-11-20", DueDate: "2026-12-20", AmountTotal: decimal.NewFromInt(300000)}
	loan, err := NewLivestockInvoiceLoan(uuid.New(), uuid.New(), inv, decimal.RequireFromString(cuppoganRate))
	require.NoError(t, err)
	require.Len(t, loan.Installments, 1)
	assert.Equal(t, "294275.53", loan.Installments[0].CapitalAmount.StringFixed(2))
	assert.Equal(t, "300000.00", loan.Installments[0].TotalAmount.StringFixed(2))
}
