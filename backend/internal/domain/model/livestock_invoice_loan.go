package model

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// A purchase on a livestock quota is invoiced in TVGAN's Odoo with the
// "Cuppogan" payment term: 3 equal installments at 30, 60 and 90 days whose
// price already includes the interest. Prestia keeps the credit without
// recomputing it: each quota invoice becomes a loan of type
// AmortizationInterestIncluded whose installments are Odoo's (amounts and due
// dates), each split into capital and interest by working the interest out of
// the price:
//
//   - the capital of the invoice is what its installments are worth today at
//     the credit line's monthly rate (their present value);
//   - the interest is the rest, spread over the installments in proportion to
//     their amounts (equal installments, equal interest).
//
// A $105 invoice in 3 installments of $35 at 1.945271 % a month (26.01 % TEA):
// capital 101.04 (33.68 per installment), interest 3.96 (1.32 per installment).
// Payments go to Odoo in full and are applied oldest installment first,
// interest before capital; an early payment pays installments in full (the
// interest is in the invoice: there is no discount).

// NewLivestockInvoiceLoan builds the active loan of a quota invoice on the
// rancher's credit line. nominalRate is the line's (annual, monthly × 12).
func NewLivestockInvoiceLoan(clientID, creditLineID uuid.UUID, inv LivestockCreditInvoice, nominalRate decimal.Decimal) (*Loan, error) {
	type part struct {
		due    time.Time
		amount decimal.Decimal
	}
	var parts []part
	for _, inst := range inv.Installments {
		due, err := time.Parse(time.DateOnly, inst.DueDate)
		if err != nil {
			return nil, fmt.Errorf("installment %d of invoice %s: due date %q: %w", inst.Number, inv.Number, inst.DueDate, err)
		}
		if inst.Amount.IsPositive() {
			parts = append(parts, part{due, inst.Amount.Round(2)})
		}
	}
	if len(parts) == 0 {
		due, err := time.Parse(time.DateOnly, inv.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invoice %s has no installments nor a due date", inv.Number)
		}
		parts = []part{{due, inv.AmountTotal.Round(2)}}
	}
	if nominalRate.IsNegative() {
		return nil, fmt.Errorf("interest rate cannot be negative")
	}

	// Present value of the installments, month k discounted k times.
	monthly, _ := nominalRate.Div(monthlyRateDivisor).Float64()
	total, presentValue := decimal.Zero, 0.0
	for k, p := range parts {
		total = total.Add(p.amount)
		a, _ := p.amount.Float64()
		presentValue += a / math.Pow(1+monthly, float64(k+1))
	}
	capital := decimal.NewFromFloat(presentValue).Round(2)
	if capital.GreaterThan(total) {
		capital = total
	}

	disbursed, err := time.Parse(time.DateOnly, inv.InvoiceDate)
	if err != nil {
		disbursed = inv.SyncedAt
	}
	loan := &Loan{
		ID: uuid.New(), ClientID: clientID, CreditLineID: creditLineID, Principal: capital,
		InterestRate: nominalRate, NumInstallments: len(parts), AmortizationType: AmortizationInterestIncluded,
		Status: LoanActive, DisbursedAt: &disbursed, ExternalReference: fmt.Sprintf("odoo-invoice:%d", inv.OdooInvoiceID),
	}
	now := time.Now()
	loan.ApprovedAt = &now
	// Each installment carries the capital in proportion to its amount; the
	// last one takes the rounding, so the capitals add up to the invoice's.
	assigned := decimal.Zero
	for i, p := range parts {
		c := p.amount.Mul(capital).Div(total).Round(2)
		if i == len(parts)-1 {
			c = capital.Sub(assigned)
		}
		assigned = assigned.Add(c)
		loan.Installments = append(loan.Installments, Installment{
			ID: uuid.New(), LoanID: loan.ID, Number: i + 1, DueDate: p.due,
			CapitalAmount: c, InterestAmount: p.amount.Sub(c), IVAAmount: decimal.Zero, TotalAmount: p.amount,
			PaidAmount: decimal.Zero, PaidCapital: decimal.Zero, PaidInterest: decimal.Zero,
			RemainingAmount: p.amount, Status: InstallmentPending,
		})
	}
	return loan, nil
}

// InvoiceTotal is what the rancher owes on the invoice: the installments'
// amounts, interest included. It is what the loan uses of the quota.
func (l *Loan) InvoiceTotal() decimal.Decimal {
	total := decimal.Zero
	for _, inst := range l.Installments {
		total = total.Add(inst.TotalAmount)
	}
	return total
}

func (i *Installment) refreshLivestockTotals() {
	i.PaidAmount = i.PaidCapital.Add(i.PaidInterest)
	i.RemainingAmount = i.TotalAmount.Sub(i.PaidAmount)
	switch {
	case !i.RemainingAmount.IsPositive():
		i.RemainingAmount = decimal.Zero
		if i.Status != InstallmentPaid {
			i.Status = InstallmentPaid
			now := time.Now()
			i.PaidAt = &now
		}
	case i.PaidAmount.IsPositive():
		i.Status = InstallmentPartial
	}
}

// LivestockPaymentSplit is how a payment was applied.
type LivestockPaymentSplit struct {
	Capital  decimal.Decimal
	Interest decimal.Decimal
}

// LivestockPayoff is the most a payment can be: everything still owed on the
// invoice (installments are paid in full, also early).
func (l *Loan) LivestockPayoff(time.Time) decimal.Decimal {
	return l.TotalRemaining()
}

// LivestockDue is what is due by `on`.
func (l *Loan) LivestockDue(on time.Time) decimal.Decimal {
	total := decimal.Zero
	for _, inst := range l.Installments {
		if inst.Status != InstallmentPaid && !inst.DueDate.After(on) {
			total = total.Add(inst.RemainingAmount)
		}
	}
	return total
}

// ApplyLivestockPayment applies a payment oldest installment first, each
// installment's interest before its capital, as Odoo applies the same amount
// to the invoice. It cannot exceed what is owed. The loan completes when
// nothing is left.
func (l *Loan) ApplyLivestockPayment(amount decimal.Decimal, on time.Time) (LivestockPaymentSplit, error) {
	split := LivestockPaymentSplit{Capital: decimal.Zero, Interest: decimal.Zero}
	if !amount.IsPositive() {
		return split, fmt.Errorf("payment amount must be positive")
	}
	if payoff := l.LivestockPayoff(on); amount.GreaterThan(payoff) {
		return split, fmt.Errorf("the payment (%s) exceeds what is owed on the invoice (%s)", amount.StringFixed(2), payoff.StringFixed(2))
	}
	left := amount
	take := func(owed decimal.Decimal) decimal.Decimal {
		if !owed.IsPositive() || !left.IsPositive() {
			return decimal.Zero
		}
		t := decimal.Min(owed, left)
		left = left.Sub(t)
		return t
	}
	for i := range l.Installments {
		inst := &l.Installments[i]
		if inst.Status == InstallmentPaid {
			continue
		}
		interest := take(inst.InterestAmount.Sub(inst.PaidInterest))
		capital := take(inst.CapitalAmount.Sub(inst.PaidCapital))
		inst.PaidInterest, inst.PaidCapital = inst.PaidInterest.Add(interest), inst.PaidCapital.Add(capital)
		split.Interest, split.Capital = split.Interest.Add(interest), split.Capital.Add(capital)
		inst.refreshLivestockTotals()
	}
	if l.CheckCompletion() && l.Status == LoanActive {
		_ = l.Complete()
	}
	return split, nil
}

// OutstandingCapital is the capital not yet paid.
func (l *Loan) OutstandingCapital() decimal.Decimal {
	total := decimal.Zero
	for _, inst := range l.Installments {
		total = total.Add(inst.CapitalAmount.Sub(inst.PaidCapital))
	}
	return total
}
