package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCreditPortfolioRolesAreAdminAndAdvisor(t *testing.T) {
	assert.ElementsMatch(t, []UserRole{RoleAdmin, RoleAdvisor}, CreditPortfolioRoles())
}

func TestOdooDocumentMatches(t *testing.T) {
	assert.True(t, OdooDocumentMatches("1000000103", "1000000103"))
	assert.True(t, OdooDocumentMatches("1.000.000.103", "1000000103"))
	assert.True(t, OdooDocumentMatches("900123456-7", "900123456"))
	assert.False(t, OdooDocumentMatches("1000000104", "1000000103"))
	assert.False(t, OdooDocumentMatches("", ""))
}

func TestLinkLifecycle(t *testing.T) {
	at := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	l := &LivestockCreditLine{ID: uuid.New()}
	assert.Equal(t, OdooLinkPending, l.Odoo.LinkStatusOrPending())
	_, ok := l.OdooPartner()
	assert.False(t, ok)

	l.MarkOdooAmbiguous([]OdooPartnerCandidate{{ID: 1}, {ID: 2}}, at)
	assert.Equal(t, OdooLinkAmbiguous, l.Odoo.LinkStatus)
	_, ok = l.OdooPartner()
	assert.False(t, ok)

	l.LinkOdooPartner(2)
	id, ok := l.OdooPartner()
	assert.True(t, ok)
	assert.EqualValues(t, 2, id)
	assert.Empty(t, l.Odoo.Candidates)

	l.RecordOdooSyncFailure("NETWORK_ERROR", "Odoo did not answer in time", at)
	assert.Equal(t, "NETWORK_ERROR", l.Odoo.LastSyncErrorCode)
	l.ApplyOdooCustomer(OdooCustomer{ID: 2, Credit: OdooCredit{ApprovedAmount: dec("5000000"), UsedAmount: dec("100000"),
		AvailableAmount: dec("4900000"), OverdueDays: 3, CanBuy: false, BlockReason: "tiene cuotas vencidas (3 días de mora)"}}, at)
	assert.Empty(t, l.Odoo.LastSyncErrorCode)
	assert.Empty(t, l.Odoo.LastSyncError)
	require.NotNil(t, l.Odoo.SnapshotAt)
	assert.Equal(t, 3, l.Odoo.OverdueDays)
	assert.True(t, l.Odoo.AvailableAmount.Equal(dec("4900000")))

	l.MarkNotInOdoo(at)
	assert.Equal(t, OdooLinkNotFound, l.Odoo.LinkStatus)
	assert.Nil(t, l.Odoo.PartnerID)
}

func invoice(number, dueDate string, residual string, installments ...LivestockCreditInstallment) LivestockCreditInvoice {
	return NewLivestockCreditInvoice(uuid.New(), OdooInvoice{ID: int64(len(number)), Number: number, Date: "2026-11-01",
		DueDate: dueDate, AmountTotal: dec("100"), AmountResidual: dec(residual), Installments: installments}, time.Now())
}

func TestInvoicesForCollection(t *testing.T) {
	inst := func(n int, due, residual string) LivestockCreditInstallment {
		return LivestockCreditInstallment{Number: n, DueDate: due, Amount: dec("33.33"), Residual: dec(residual)}
	}
	// Due last overall, but its oldest unpaid installment is due first.
	threeInstallments := invoice("FE12262", "2027-02-10", "66.67",
		inst(1, "2026-10-12", "0"), inst(2, "2026-11-11", "33.33"), inst(3, "2026-12-10", "33.34"))
	overdue := invoice("FE12263", "2026-11-20", "100")
	paid := invoice("FE12100", "2026-10-01", "0")

	assert.Equal(t, "2026-11-11", threeInstallments.NextDueDate())
	assert.Equal(t, "2026-11-20", overdue.NextDueDate())
	assert.False(t, paid.IsOpen())

	list := []LivestockCreditInvoice{paid, overdue, threeInstallments}
	SortInvoicesForCollection(list)
	assert.Equal(t, []string{"FE12262", "FE12263", "FE12100"}, []string{list[0].Number, list[1].Number, list[2].Number})
	assert.Equal(t, "FE12262", OldestOpenInvoice(list).Number)
	assert.Nil(t, OldestOpenInvoice([]LivestockCreditInvoice{paid}))
}

func TestPaymentRequestValidation(t *testing.T) {
	valid := func() LivestockCreditPaymentRequest {
		return LivestockCreditPaymentRequest{InvoiceID: 27507, Amount: dec("100000"), Bank: " davivienda ", Reference: " PRESTIA-1 "}
	}
	r := valid()
	require.NoError(t, r.Validate("2026-11-16"))
	assert.Equal(t, "2026-11-16", r.Date, "empty date is today")
	assert.Equal(t, "DAVIVIENDA", r.Bank)
	assert.Equal(t, "PRESTIA-1", r.Reference)

	cases := map[string]func(*LivestockCreditPaymentRequest){
		"invoice_required":   func(r *LivestockCreditPaymentRequest) { r.InvoiceID = 0 },
		"invalid_amount":     func(r *LivestockCreditPaymentRequest) { r.Amount = dec("0") },
		"reference_required": func(r *LivestockCreditPaymentRequest) { r.Reference = "  " },
		"bank_required":      func(r *LivestockCreditPaymentRequest) { r.Bank = "" },
		"invalid_date":       func(r *LivestockCreditPaymentRequest) { r.Date = "16/11/2026" },
		"future_date":        func(r *LivestockCreditPaymentRequest) { r.Date = "2026-11-17" },
	}
	for code, mutate := range cases {
		r := valid()
		mutate(&r)
		err := r.Validate("2026-11-16")
		var rej *PortfolioRejection
		require.ErrorAs(t, err, &rej, code)
		assert.Equal(t, code, rej.Code)
		assert.Equal(t, PortfolioInvalid, rej.Kind)
	}
	r = valid()
	r.Amount = dec("10.005")
	assert.Error(t, r.Validate("2026-11-16"), "more than two decimals")
	r = valid()
	r.Amount = dec("825260.80")
	assert.NoError(t, r.Validate("2026-11-16"))
}

func TestPaymentOutcomes(t *testing.T) {
	lineID := uuid.New()
	req := LivestockCreditPaymentRequest{InvoiceID: 1, Amount: dec("10"), Date: "2026-11-16", Bank: "BANCOLOMBIA", Reference: "R"}
	p := NewLivestockCreditPayment(lineID, req, "FE1", nil, "staff@example.com")
	assert.True(t, p.Replays(lineID, req))
	other := req
	other.Amount = dec("11")
	assert.False(t, p.Replays(lineID, other))

	p.MarkAlreadyRegistered("PAYMENT_ALREADY_REGISTERED", "Ya se registró un pago con la referencia R (pago PBCOL/2026/01768).")
	assert.Equal(t, PaymentAlreadyRegistered, p.Status)
	assert.Equal(t, "PBCOL/2026/01768", p.OdooPaymentNumber)

	p.MarkRegistered(2112, "PBCOL/2026/01769")
	assert.Equal(t, PaymentRegistered, p.Status)
	assert.EqualValues(t, 2112, *p.OdooPaymentID)
	assert.Empty(t, p.ErrorCode)

	p.MarkFailed("INVALID_BANK", "bank debe ser: BANCOLOMBIA o DAVIVIENDA.")
	assert.Equal(t, PaymentFailed, p.Status)
}

func TestBlockEventNeedsAReason(t *testing.T) {
	_, err := NewLivestockCreditBlockEvent(uuid.New(), true, "   ", nil, "")
	var rej *PortfolioRejection
	require.ErrorAs(t, err, &rej)
	assert.Equal(t, "reason_required", rej.Code)
	e, err := NewLivestockCreditBlockEvent(uuid.New(), false, " acuerdo de pago ", nil, "a@b.co")
	require.NoError(t, err)
	assert.Equal(t, "acuerdo de pago", e.Reason)
	assert.False(t, e.Blocked)
}

func TestTruncateKeepsRunes(t *testing.T) {
	assert.Equal(t, "dí", truncate("días", 3))
	assert.Equal(t, "abc", truncate(" abc ", 10))
}
