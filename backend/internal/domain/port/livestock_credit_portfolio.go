package port

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OdooPortfolioGateway is TVGAN's Odoo as the ledger of the livestock credit
// (API /api/v1/cuppogan). Failures are *OdooError.
type OdooPortfolioGateway interface {
	// SearchCustomers finds the contacts with a document (cédula or NIT, with
	// or without verification digit). TVGAN's base has repeated documents, so
	// it may return several.
	SearchCustomers(ctx context.Context, document string) ([]model.OdooCustomer, error)
	GetCustomer(ctx context.Context, partnerID int64) (*model.OdooCustomer, error)
	// CustomerInvoices returns every quota invoice of a contact (open and
	// paid) and whether the list is complete (Odoo caps the page).
	CustomerInvoices(ctx context.Context, partnerID int64) ([]model.OdooInvoice, bool, error)
	GetInvoice(ctx context.Context, invoiceID int64) (*model.OdooInvoice, error)
	Banks(ctx context.Context) ([]model.OdooBank, error)
	// RegisterPayment creates the payment in Odoo, which applies it to the
	// invoice's oldest installment. The reference is unique: a second time
	// answers 409 PAYMENT_ALREADY_REGISTERED and duplicates nothing.
	RegisterPayment(ctx context.Context, invoiceID int64, p model.OdooPaymentRequest) (*model.OdooPaymentResult, error)
	// SetBlocked sends only credit.blocked: the other credit fields belong to
	// CuppoGan's service.
	SetBlocked(ctx context.Context, partnerID int64, blocked bool) (*model.OdooCustomer, error)
	// RemainingBudget is how many requests the client-side throttle still
	// allows in its tightest window. The background sync leaves a reserve for
	// staff's requests.
	RemainingBudget() int
}

// Odoo error codes the portfolio acts on.
const (
	OdooPaymentAlreadyRegistered = "PAYMENT_ALREADY_REGISTERED"
	OdooCustomerNotFound         = "CUSTOMER_NOT_FOUND"
	OdooInvoiceNotFound          = "INVOICE_NOT_FOUND"
)

// OdooError is an Odoo answer that is not a success ({"error", "code"} with
// its HTTP status), or Odoo not answering at all (Status 0).
type OdooError struct {
	Status  int
	Code    string
	Message string
}

func (e *OdooError) Error() string {
	return fmt.Sprintf("odoo %d %s: %s", e.Status, e.Code, e.Message)
}

// Unavailable tells whether the failure is Odoo's or the link's, not the
// request's: no answer, a server error, the rate limit, or Prestia's
// credentials or IP refused. Nothing was done and the same request can be
// retried once it is solved.
func (e *OdooError) Unavailable() bool {
	return e.Status == 0 || e.Status >= 500 || e.Status == 429 || e.Status == 401 || e.Status == 403
}

// LivestockCreditPortfolioRepository stores the Odoo side of the credit lines:
// the link and snapshot (in the line's row), the invoices, the payments staff
// registered and the block history.
type LivestockCreditPortfolioRepository interface {
	// SaveOdooState writes the line's Odoo columns only.
	SaveOdooState(ctx context.Context, line *model.LivestockCreditLine) error
	// ListLinesForSync returns active lines that can be synced unattended
	// (not ambiguous), least recently synced first.
	ListLinesForSync(ctx context.Context, limit int) ([]model.LivestockCreditLine, error)

	// UpsertInvoices stores invoices by Odoo id. With complete, the line's
	// invoices Odoo no longer reports are removed.
	UpsertInvoices(ctx context.Context, lineID uuid.UUID, invoices []model.LivestockCreditInvoice, complete bool) error
	ListInvoices(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditInvoice, error)

	// OpenInvoiceLoan stores the loan of a quota invoice (with its
	// installments), adds its principal to the credit line's used amount and
	// links the invoice to it, together or not at all.
	OpenInvoiceLoan(ctx context.Context, invoiceID uuid.UUID, loan *model.Loan) error
	// RecordLoanPayment stores a payment applied to an invoice's loan: the
	// installments and status of the loan, Prestia's payment, the capital
	// released from the credit line and the portfolio payment, together.
	RecordLoanPayment(ctx context.Context, loan *model.Loan, loanPayment *model.Payment, capital decimal.Decimal, payment *model.LivestockCreditPayment) error

	// CreatePayment fails if another payment that is not failed has the
	// reference (unique index).
	CreatePayment(ctx context.Context, p *model.LivestockCreditPayment) error
	// FindActivePaymentByReference finds the payment, not failed, with a
	// reference; nil, nil when none.
	FindActivePaymentByReference(ctx context.Context, reference string) (*model.LivestockCreditPayment, error)
	// ListPayments returns a line's payments, newest first.
	ListPayments(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditPayment, error)

	CreateBlockEvent(ctx context.Context, e *model.LivestockCreditBlockEvent) error
	// ListBlockEvents returns a line's block history, newest first.
	ListBlockEvents(ctx context.Context, lineID uuid.UUID) ([]model.LivestockCreditBlockEvent, error)
}
