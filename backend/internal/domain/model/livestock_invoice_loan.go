package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// A purchase on a livestock quota is invoiced in TVGAN's Odoo with the
// "Cuppogan" payment term: equal installments of capital at 30, 60 and 90 days.
// Prestia keeps the credit: each quota invoice becomes a loan whose installments
// carry Odoo's capital and due dates (so both systems agree on what is due and
// when) plus the interest of the credit line's rate on the outstanding capital,
// German system (constant capital). Odoo only knows the capital: a payment's
// capital is reported to Odoo, its interest stays in Prestia.

// NewLivestockInvoiceLoan builds the active loan of a quota invoice on the
// rancher's credit line. nominalRate is the line's (annual, monthly × 12).
func NewLivestockInvoiceLoan(clientID, creditLineID uuid.UUID, inv LivestockCreditInvoice, nominalRate decimal.Decimal) (*Loan, error) {
	type part struct {
		due     time.Time
		capital decimal.Decimal
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
	principal := decimal.Zero
	for _, p := range parts {
		principal = principal.Add(p.capital)
	}
	loan, err := NewLoan(clientID, creditLineID, principal, nominalRate, len(parts), AmortizationGerman)
	if err != nil {
		return nil, err
	}
	if err := loan.RequestApproval(); err != nil {
		return nil, err
	}
	if err := loan.ApproveForPartner(); err != nil {
		return nil, err
	}
	disbursed, err := time.Parse(time.DateOnly, inv.InvoiceDate)
	if err != nil {
		disbursed = inv.SyncedAt
	}
	loan.Status, loan.DisbursedAt = LoanActive, &disbursed
	for i, p := range parts {
		loan.Installments = append(loan.Installments, Installment{
			ID: uuid.New(), LoanID: loan.ID, Number: i + 1, DueDate: p.due, CapitalAmount: p.capital,
			IVAAmount: decimal.Zero, PaidAmount: decimal.Zero, PaidCapital: decimal.Zero, PaidInterest: decimal.Zero,
			Status: InstallmentPending,
		})
	}
	loan.recomputeLivestockInterest(time.Time{})
	return loan, nil
}

// monthlyRate is the loan's monthly rate as a fraction.
func (l *Loan) monthlyRate() decimal.Decimal {
	return l.InterestRate.Div(monthlyRateDivisor)
}

// recomputeLivestockInterest sets the interest of every installment not yet
// due on `asOf` (all of them, with a zero asOf) to the monthly rate on the
// capital still owed from that installment on. Paid interest is never undone.
func (l *Loan) recomputeLivestockInterest(asOf time.Time) {
	rate := l.monthlyRate()
	balance := decimal.Zero
	for i := len(l.Installments) - 1; i >= 0; i-- {
		inst := &l.Installments[i]
		balance = balance.Add(inst.CapitalAmount.Sub(inst.PaidCapital))
		if inst.Status == InstallmentPaid || (!asOf.IsZero() && !inst.DueDate.After(asOf)) {
			continue
		}
		interest := balance.Mul(rate).Round(2)
		if interest.LessThan(inst.PaidInterest) {
			interest = inst.PaidInterest
		}
		inst.InterestAmount = interest
		inst.refreshLivestockTotals()
	}
}

func (i *Installment) refreshLivestockTotals() {
	i.TotalAmount = i.CapitalAmount.Add(i.InterestAmount)
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

// LivestockPayoff is the most a payment made on `on` can be: what is due by
// then (capital and interest) plus the capital not yet due.
func (l *Loan) LivestockPayoff(on time.Time) decimal.Decimal {
	total := decimal.Zero
	for _, inst := range l.Installments {
		if inst.Status == InstallmentPaid {
			continue
		}
		if inst.DueDate.After(on) {
			total = total.Add(inst.CapitalAmount.Sub(inst.PaidCapital))
		} else {
			total = total.Add(inst.RemainingAmount)
		}
	}
	return total
}

// LivestockDue is what is due by `on` (capital and interest of the due installments).
func (l *Loan) LivestockDue(on time.Time) decimal.Decimal {
	total := decimal.Zero
	for _, inst := range l.Installments {
		if inst.Status != InstallmentPaid && !inst.DueDate.After(on) {
			total = total.Add(inst.RemainingAmount)
		}
	}
	return total
}

// ApplyLivestockPayment applies a payment made on `on`: first the installments
// due by then, oldest first, interest before capital; the rest prepays capital,
// oldest installment first (as Odoo applies it), and the interest of the
// installments not yet due is recomputed on the new balance. It cannot exceed
// LivestockPayoff. The loan completes when nothing is left.
func (l *Loan) ApplyLivestockPayment(amount decimal.Decimal, on time.Time) (LivestockPaymentSplit, error) {
	split := LivestockPaymentSplit{Capital: decimal.Zero, Interest: decimal.Zero}
	if !amount.IsPositive() {
		return split, fmt.Errorf("payment amount must be positive")
	}
	if payoff := l.LivestockPayoff(on); amount.GreaterThan(payoff) {
		return split, fmt.Errorf("the payment (%s) exceeds what can be paid on the loan (%s)", amount.StringFixed(2), payoff.StringFixed(2))
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
		if inst.Status == InstallmentPaid || inst.DueDate.After(on) {
			continue
		}
		interest := take(inst.InterestAmount.Sub(inst.PaidInterest))
		capital := take(inst.CapitalAmount.Sub(inst.PaidCapital))
		inst.PaidInterest, inst.PaidCapital = inst.PaidInterest.Add(interest), inst.PaidCapital.Add(capital)
		split.Interest, split.Capital = split.Interest.Add(interest), split.Capital.Add(capital)
		inst.refreshLivestockTotals()
	}
	for i := range l.Installments {
		inst := &l.Installments[i]
		if inst.Status == InstallmentPaid || !inst.DueDate.After(on) {
			continue
		}
		capital := take(inst.CapitalAmount.Sub(inst.PaidCapital))
		inst.PaidCapital = inst.PaidCapital.Add(capital)
		split.Capital = split.Capital.Add(capital)
	}
	l.recomputeLivestockInterest(on)
	for i := range l.Installments {
		l.Installments[i].refreshLivestockTotals()
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
