package dto

import (
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
)

// PortfolioErrorResponse is a portfolio operation that was not done. Code is
// Prestia's stable code; OdooCode is Odoo's own when Odoo answered; Retryable
// says the same request can be sent again (Odoo was unavailable and nothing
// was stored).
type PortfolioErrorResponse struct {
	Code      string `json:"code" example:"odoo_rejected"`
	Error     string `json:"error"`
	OdooCode  string `json:"odooCode,omitempty" example:"AMOUNT_EXCEEDS_RESIDUAL"`
	Retryable bool   `json:"retryable"`
}

// LivestockCreditLineOdooSummary is what the credit line list shows of Odoo.
type LivestockCreditLineOdooSummary struct {
	// LinkStatus: pending, linked, not_in_odoo or ambiguous.
	LinkStatus string `json:"linkStatus" example:"linked"`
	// Synced is whether Odoo's credit snapshot was ever read.
	Synced      bool       `json:"synced"`
	OverdueDays int        `json:"overdueDays"`
	Blocked     bool       `json:"blocked"`
	CanBuy      bool       `json:"canBuy"`
	SnapshotAt  *time.Time `json:"snapshotAt,omitempty"`
}

func ToLivestockCreditLineOdooSummary(o model.LivestockCreditLineOdoo) LivestockCreditLineOdooSummary {
	return LivestockCreditLineOdooSummary{
		LinkStatus: string(o.LinkStatusOrPending()), Synced: o.SnapshotAt != nil,
		OverdueDays: o.OverdueDays, Blocked: o.Blocked, CanBuy: o.CanBuy, SnapshotAt: o.SnapshotAt,
	}
}

// OdooPartnerCandidateResponse is an Odoo contact an ambiguous national ID
// matched.
type OdooPartnerCandidateResponse struct {
	ID       int64  `json:"id" example:"71817"`
	Name     string `json:"name"`
	Document string `json:"document"`
	City     string `json:"city"`
	State    string `json:"state"`
}

// LivestockCreditOdooCredit is the credit block as Odoo computed it. Amounts
// are decimal strings: Odoo's figures, never recomputed by Prestia.
type LivestockCreditOdooCredit struct {
	ValidationStatus   string    `json:"validationStatus" example:"aprobado"`
	Approved           bool      `json:"approved"`
	ApprovedAmount     string    `json:"approvedAmount" example:"5000000.00"`
	UsedAmount         string    `json:"usedAmount" example:"2576030.00"`
	AvailableAmount    string    `json:"availableAmount" example:"2423970.00"`
	OverdueDays        int       `json:"overdueDays"`
	Blocked            bool      `json:"blocked"`
	CanBuy             bool      `json:"canBuy"`
	BlockReason        string    `json:"blockReason" example:"tiene cuotas vencidas (12 días de mora)"`
	PromissoryNoteDate string    `json:"promissoryNoteDate" example:"2026-11-12"`
	LastUpdate         string    `json:"lastUpdate" example:"2026-11-12 13:09:58"`
	SnapshotAt         time.Time `json:"snapshotAt"`
}

// LivestockCreditOdooState is the line's link to Odoo and its last snapshot.
type LivestockCreditOdooState struct {
	// Enabled is whether Prestia has an Odoo token; without it the stored data
	// is shown and nothing can be sent.
	Enabled    bool                           `json:"enabled"`
	LinkStatus string                         `json:"linkStatus" example:"linked"`
	PartnerID  *int64                         `json:"partnerId,omitempty" example:"71817"`
	Candidates []OdooPartnerCandidateResponse `json:"candidates"`
	// Credit is absent until the first successful read.
	Credit            *LivestockCreditOdooCredit `json:"credit,omitempty"`
	LastSyncAt        *time.Time                 `json:"lastSyncAt,omitempty"`
	LastSyncError     string                     `json:"lastSyncError,omitempty"`
	LastSyncErrorCode string                     `json:"lastSyncErrorCode,omitempty"`
}

// LivestockCreditInstallmentResponse is one installment of an invoice.
type LivestockCreditInstallmentResponse struct {
	Number   int    `json:"number"`
	DueDate  string `json:"dueDate" example:"2026-12-12"`
	Amount   string `json:"amount" example:"825260.80"`
	Residual string `json:"residual" example:"825260.80"`
}

// LivestockCreditInvoiceResponse is a quota invoice as Odoo last reported it.
type LivestockCreditInvoiceResponse struct {
	OdooInvoiceID  int64  `json:"odooInvoiceId" example:"27506"`
	Number         string `json:"number" example:"FE12262"`
	Order          string `json:"order" example:"COT02404"`
	Date           string `json:"date" example:"2026-11-12"`
	DueDate        string `json:"dueDate" example:"2027-02-10"`
	AmountTotal    string `json:"amountTotal" example:"2476030.00"`
	AmountResidual string `json:"amountResidual" example:"2476030.00"`
	// PaymentState: not_paid, in_payment, partial, paid or reversed.
	PaymentState string `json:"paymentState" example:"not_paid"`
	Open         bool   `json:"open"`
	// NextDueDate is the due date of the oldest installment with a balance.
	NextDueDate  string                               `json:"nextDueDate" example:"2026-12-12"`
	Installments []LivestockCreditInstallmentResponse `json:"installments"`
	SyncedAt     time.Time                            `json:"syncedAt"`
}

// LivestockCreditPaymentResponse is a payment staff registered.
type LivestockCreditPaymentResponse struct {
	ID                string    `json:"id"`
	OdooInvoiceID     int64     `json:"odooInvoiceId"`
	InvoiceNumber     string    `json:"invoiceNumber" example:"FE12262"`
	Amount            string    `json:"amount" example:"825260.80"`
	Date              string    `json:"date" example:"2026-11-16"`
	Bank              string    `json:"bank" example:"BANCOLOMBIA"`
	Reference         string    `json:"reference" example:"TRX-20261116-001"`
	Status            string    `json:"status" example:"registered"`
	OdooPaymentID     *int64    `json:"odooPaymentId,omitempty"`
	OdooPaymentNumber string    `json:"odooPaymentNumber" example:"PBCOL/2026/01768"`
	ErrorCode         string    `json:"errorCode,omitempty"`
	ErrorMessage      string    `json:"errorMessage,omitempty"`
	RegisteredBy      string    `json:"registeredBy"`
	CreatedAt         time.Time `json:"createdAt"`
}

// LivestockCreditBlockEventResponse is a block or unblock decided in Prestia.
type LivestockCreditBlockEventResponse struct {
	ID        string    `json:"id"`
	Blocked   bool      `json:"blocked"`
	Reason    string    `json:"reason"`
	By        string    `json:"by"`
	CreatedAt time.Time `json:"createdAt"`
}

// LivestockCreditPortfolioResponse is the Odoo side of a credit line.
type LivestockCreditPortfolioResponse struct {
	CreditLineID string                   `json:"creditLineId"`
	Odoo         LivestockCreditOdooState `json:"odoo"`
	// Invoices: open ones first, by the due date of their oldest unpaid
	// installment, then the paid ones.
	Invoices []LivestockCreditInvoiceResponse `json:"invoices"`
	// SuggestedInvoiceID is the open invoice due first, which the payment
	// form preselects.
	SuggestedInvoiceID *int64                              `json:"suggestedInvoiceId,omitempty"`
	Payments           []LivestockCreditPaymentResponse    `json:"payments"`
	BlockEvents        []LivestockCreditBlockEventResponse `json:"blockEvents"`
}

// RegisterLivestockCreditPaymentRequest is a payment to send to Odoo.
type RegisterLivestockCreditPaymentRequest struct {
	InvoiceID int64           `json:"invoiceId" example:"27506"`
	Amount    decimal.Decimal `json:"amount" swaggertype:"string" example:"825260.80"`
	// Date is YYYY-MM-DD, today when empty; never in the future.
	Date      string `json:"date" example:"2026-11-16"`
	Bank      string `json:"bank" example:"BANCOLOMBIA"`
	Reference string `json:"reference" example:"TRX-20261116-001"`
}

// LivestockCreditPaymentResult is a payment's outcome and the portfolio after
// it.
type LivestockCreditPaymentResult struct {
	Payment LivestockCreditPaymentResponse `json:"payment"`
	// Replayed is a double submission answered with the payment already
	// stored; nothing was sent to Odoo.
	Replayed  bool                             `json:"replayed"`
	Portfolio LivestockCreditPortfolioResponse `json:"portfolio"`
}

// LinkOdooPartnerRequest is the Odoo contact staff picks for an ambiguous
// national ID.
type LinkOdooPartnerRequest struct {
	PartnerID int64 `json:"partnerId" example:"71817"`
}

// SetLivestockCreditBlockRequest blocks or unblocks a rancher's purchases on
// the quota.
type SetLivestockCreditBlockRequest struct {
	Blocked *bool  `json:"blocked" example:"true"`
	Reason  string `json:"reason" example:"Acuerdo de pago incumplido"`
}

// OdooBankResponse is a TVGAN account Odoo takes payments into.
type OdooBankResponse struct {
	Bank    string `json:"bank" example:"BANCOLOMBIA"`
	Journal string `json:"journal" example:"Bancolombia"`
	Account string `json:"account"`
}

// OdooBankList is the banks the payment form offers.
type OdooBankList struct {
	Banks []OdooBankResponse `json:"banks"`
}

func ToOdooBankList(banks []model.OdooBank) OdooBankList {
	out := OdooBankList{Banks: make([]OdooBankResponse, len(banks))}
	for i, b := range banks {
		out.Banks[i] = OdooBankResponse{Bank: b.Bank, Journal: b.Journal, Account: b.Account}
	}
	return out
}

func ToLivestockCreditOdooState(enabled bool, o model.LivestockCreditLineOdoo) LivestockCreditOdooState {
	s := LivestockCreditOdooState{
		Enabled: enabled, LinkStatus: string(o.LinkStatusOrPending()),
		Candidates: make([]OdooPartnerCandidateResponse, len(o.Candidates)),
		LastSyncAt: o.LastSyncAt, LastSyncError: o.LastSyncError, LastSyncErrorCode: o.LastSyncErrorCode,
	}
	if o.LinkStatusOrPending() == model.OdooLinkLinked {
		s.PartnerID = o.PartnerID
	}
	for i, c := range o.Candidates {
		s.Candidates[i] = OdooPartnerCandidateResponse{ID: c.ID, Name: c.Name, Document: c.Document, City: c.City, State: c.State}
	}
	if o.SnapshotAt != nil {
		s.Credit = &LivestockCreditOdooCredit{
			ValidationStatus: o.ValidationStatus, Approved: o.Approved,
			ApprovedAmount: o.ApprovedAmount.StringFixed(2), UsedAmount: o.UsedAmount.StringFixed(2),
			AvailableAmount: o.AvailableAmount.StringFixed(2), OverdueDays: o.OverdueDays,
			Blocked: o.Blocked, CanBuy: o.CanBuy, BlockReason: o.BlockReason,
			PromissoryNoteDate: o.PromissoryNoteDate, LastUpdate: o.CreditUpdatedAt, SnapshotAt: *o.SnapshotAt,
		}
	}
	return s
}

func ToLivestockCreditInvoiceResponse(i model.LivestockCreditInvoice) LivestockCreditInvoiceResponse {
	r := LivestockCreditInvoiceResponse{
		OdooInvoiceID: i.OdooInvoiceID, Number: i.Number, Order: i.OrderRef, Date: i.InvoiceDate,
		DueDate: i.DueDate, AmountTotal: i.AmountTotal.StringFixed(2), AmountResidual: i.AmountResidual.StringFixed(2),
		PaymentState: i.PaymentState, Open: i.IsOpen(), NextDueDate: i.NextDueDate(),
		Installments: make([]LivestockCreditInstallmentResponse, len(i.Installments)), SyncedAt: i.SyncedAt,
	}
	for k, inst := range i.Installments {
		r.Installments[k] = LivestockCreditInstallmentResponse{Number: inst.Number, DueDate: inst.DueDate,
			Amount: inst.Amount.StringFixed(2), Residual: inst.Residual.StringFixed(2)}
	}
	return r
}

func ToLivestockCreditPaymentResponse(p model.LivestockCreditPayment) LivestockCreditPaymentResponse {
	return LivestockCreditPaymentResponse{
		ID: p.ID.String(), OdooInvoiceID: p.OdooInvoiceID, InvoiceNumber: p.InvoiceNumber,
		Amount: p.Amount.StringFixed(2), Date: p.PaymentDate, Bank: p.Bank, Reference: p.Reference,
		Status: string(p.Status), OdooPaymentID: p.OdooPaymentID, OdooPaymentNumber: p.OdooPaymentNumber,
		ErrorCode: p.ErrorCode, ErrorMessage: p.ErrorMessage, RegisteredBy: p.RegisteredByEmail, CreatedAt: p.CreatedAt,
	}
}

func ToLivestockCreditPortfolioResponse(enabled bool, line *model.LivestockCreditLine, invoices []model.LivestockCreditInvoice,
	payments []model.LivestockCreditPayment, events []model.LivestockCreditBlockEvent, suggested *model.LivestockCreditInvoice) LivestockCreditPortfolioResponse {
	r := LivestockCreditPortfolioResponse{
		CreditLineID: line.ID.String(),
		Odoo:         ToLivestockCreditOdooState(enabled, line.Odoo),
		Invoices:     make([]LivestockCreditInvoiceResponse, len(invoices)),
		Payments:     make([]LivestockCreditPaymentResponse, len(payments)),
		BlockEvents:  make([]LivestockCreditBlockEventResponse, len(events)),
	}
	for i := range invoices {
		r.Invoices[i] = ToLivestockCreditInvoiceResponse(invoices[i])
	}
	for i := range payments {
		r.Payments[i] = ToLivestockCreditPaymentResponse(payments[i])
	}
	for i, e := range events {
		r.BlockEvents[i] = LivestockCreditBlockEventResponse{ID: e.ID.String(), Blocked: e.Blocked, Reason: e.Reason,
			By: e.UserEmail, CreatedAt: e.CreatedAt}
	}
	if suggested != nil {
		id := suggested.OdooInvoiceID
		r.SuggestedInvoiceID = &id
	}
	return r
}
