package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CreditPortfolioRoles are the roles that work the portfolio (cartera) of the
// livestock credit lines: they see the Odoo snapshot, register payments and
// block or unblock a rancher. Today the same group as the credit line pages;
// narrow it here when a dedicated portfolio role exists.
func CreditPortfolioRoles() []UserRole {
	return []UserRole{RoleAdmin, RoleAdvisor}
}

// TVGAN's Odoo is the ledger of the livestock credit: it invoices the purchases
// made on the quota (three installments), takes the payments and computes the
// used and available amounts and the days overdue. Prestia keeps a snapshot of
// what Odoo answers, to show it, and never computes a balance of its own.

// OdooLinkStatus is how a credit line is matched to its Odoo contact.
type OdooLinkStatus string

const (
	// OdooLinkPending is a line not yet looked up in Odoo.
	OdooLinkPending OdooLinkStatus = "pending"
	// OdooLinkLinked is a line matched to one Odoo contact.
	OdooLinkLinked OdooLinkStatus = "linked"
	// OdooLinkNotFound is a line whose national ID matches no Odoo contact.
	// It is searched again on every sync.
	OdooLinkNotFound OdooLinkStatus = "not_in_odoo"
	// OdooLinkAmbiguous is a line whose national ID matches several Odoo
	// contacts (TVGAN's base has repeated documents): staff picks one.
	OdooLinkAmbiguous OdooLinkStatus = "ambiguous"
)

// OdooPartnerCandidate is one of the Odoo contacts an ambiguous line matched.
type OdooPartnerCandidate struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Document string `json:"document"`
	City     string `json:"city"`
	State    string `json:"state"`
}

// LivestockCreditLineOdoo is the line's link to its Odoo contact and the last
// credit snapshot Odoo gave for it. Stored in the line's own row (columns
// prefixed odoo_) so the list shows overdue days and blocks without a join.
type LivestockCreditLineOdoo struct {
	LinkStatus OdooLinkStatus         `gorm:"type:varchar(20);default:'pending'"`
	PartnerID  *int64                 `gorm:"index"`
	Candidates []OdooPartnerCandidate `gorm:"serializer:json;type:text"`

	// The credit block as Odoo computed it on SnapshotAt.
	ValidationStatus   string          `gorm:"type:varchar(20);default:''"`
	Approved           bool            `gorm:"default:false"`
	ApprovedAmount     decimal.Decimal `gorm:"type:decimal(18,2);default:0"`
	UsedAmount         decimal.Decimal `gorm:"type:decimal(18,2);default:0"`
	AvailableAmount    decimal.Decimal `gorm:"type:decimal(18,2);default:0"`
	OverdueDays        int             `gorm:"default:0"`
	Blocked            bool            `gorm:"default:false"`
	CanBuy             bool            `gorm:"default:false"`
	BlockReason        string          `gorm:"type:varchar(300);default:''"`
	PromissoryNoteDate string          `gorm:"type:varchar(10);default:''"`
	// CreditUpdatedAt is Odoo's last_update (UTC, as Odoo formats it).
	CreditUpdatedAt string `gorm:"type:varchar(30);default:''"`
	SnapshotAt      *time.Time

	// LastSyncAt is the last attempt; LastSyncError(Code) its failure, empty
	// when it succeeded.
	LastSyncAt        *time.Time
	LastSyncError     string `gorm:"type:varchar(500);default:''"`
	LastSyncErrorCode string `gorm:"type:varchar(60);default:''"`
}

// OdooCredit is the credit block of an Odoo contact.
type OdooCredit struct {
	ValidationStatus   string
	Approved           bool
	ApprovedAmount     decimal.Decimal
	UsedAmount         decimal.Decimal
	AvailableAmount    decimal.Decimal
	PromissoryNoteDate string
	Blocked            bool
	OverdueDays        int
	LastUpdate         string
	CanBuy             bool
	BlockReason        string
}

// OdooCustomer is a contact of TVGAN's Odoo with its credit block.
type OdooCustomer struct {
	ID       int64
	Name     string
	Document string
	City     string
	State    string
	Credit   OdooCredit
}

// OdooInvoice is an invoice on the quota (payment term «Cuppogan»).
type OdooInvoice struct {
	ID             int64
	Number         string
	Date           string
	DueDate        string
	CustomerID     int64
	Order          string
	AmountTotal    decimal.Decimal
	AmountResidual decimal.Decimal
	PaymentState   string
	Installments   []LivestockCreditInstallment
}

// OdooBank is a TVGAN bank account Odoo accepts payments into.
type OdooBank struct {
	Bank    string
	Journal string
	Account string
}

// OdooPaymentRequest is a payment sent to Odoo for one invoice.
type OdooPaymentRequest struct {
	Amount    decimal.Decimal
	Date      string
	Bank      string
	Reference string
}

// OdooPaymentResult is Odoo's answer to a registered payment: the payment and
// the invoice and contact as they are after it.
type OdooPaymentResult struct {
	PaymentID     int64
	PaymentNumber string
	Invoice       *OdooInvoice
	Customer      *OdooCustomer
}

// LinkStatusOrPending is the link status, pending for rows stored before the
// column existed.
func (o LivestockCreditLineOdoo) LinkStatusOrPending() OdooLinkStatus {
	if o.LinkStatus == "" {
		return OdooLinkPending
	}
	return o.LinkStatus
}

// OdooPartner returns the Odoo contact the line is linked to.
func (l *LivestockCreditLine) OdooPartner() (int64, bool) {
	if l.Odoo.LinkStatusOrPending() != OdooLinkLinked || l.Odoo.PartnerID == nil {
		return 0, false
	}
	return *l.Odoo.PartnerID, true
}

// LinkOdooPartner matches the line to an Odoo contact.
func (l *LivestockCreditLine) LinkOdooPartner(partnerID int64) {
	id := partnerID
	l.Odoo.LinkStatus = OdooLinkLinked
	l.Odoo.PartnerID = &id
	l.Odoo.Candidates = nil
}

// MarkNotInOdoo records that no Odoo contact has the line's national ID.
func (l *LivestockCreditLine) MarkNotInOdoo(at time.Time) {
	l.Odoo.LinkStatus = OdooLinkNotFound
	l.Odoo.PartnerID = nil
	l.Odoo.Candidates = nil
	l.markSynced(at)
}

// MarkOdooAmbiguous records the several Odoo contacts the national ID matched.
func (l *LivestockCreditLine) MarkOdooAmbiguous(candidates []OdooPartnerCandidate, at time.Time) {
	l.Odoo.LinkStatus = OdooLinkAmbiguous
	l.Odoo.PartnerID = nil
	l.Odoo.Candidates = candidates
	l.markSynced(at)
}

// ApplyOdooCustomer takes the credit snapshot Odoo answered for the contact.
func (l *LivestockCreditLine) ApplyOdooCustomer(c OdooCustomer, at time.Time) {
	o := &l.Odoo
	o.ValidationStatus = c.Credit.ValidationStatus
	o.Approved = c.Credit.Approved
	o.ApprovedAmount = c.Credit.ApprovedAmount.Round(2)
	o.UsedAmount = c.Credit.UsedAmount.Round(2)
	o.AvailableAmount = c.Credit.AvailableAmount.Round(2)
	o.OverdueDays = c.Credit.OverdueDays
	o.Blocked = c.Credit.Blocked
	o.CanBuy = c.Credit.CanBuy
	o.BlockReason = truncate(c.Credit.BlockReason, 300)
	o.PromissoryNoteDate = truncate(c.Credit.PromissoryNoteDate, 10)
	o.CreditUpdatedAt = truncate(c.Credit.LastUpdate, 30)
	snapshot := at.UTC()
	o.SnapshotAt = &snapshot
	l.markSynced(at)
}

// RecordOdooSyncFailure keeps why the last sync failed; the snapshot stays.
func (l *LivestockCreditLine) RecordOdooSyncFailure(code, message string, at time.Time) {
	t := at.UTC()
	l.Odoo.LastSyncAt = &t
	l.Odoo.LastSyncErrorCode = truncate(code, 60)
	l.Odoo.LastSyncError = truncate(message, 500)
}

func (l *LivestockCreditLine) markSynced(at time.Time) {
	t := at.UTC()
	l.Odoo.LastSyncAt = &t
	l.Odoo.LastSyncError = ""
	l.Odoo.LastSyncErrorCode = ""
}

// OdooDocumentMatches tells whether an Odoo contact's document is the line's
// national ID. Odoo keeps NITs with or without the verification digit
// ("900123456-7"), so only the digits before a dash are compared.
func OdooDocumentMatches(odooDocument, nationalID string) bool {
	normalize := func(s string) string {
		if i := strings.Index(s, "-"); i >= 0 {
			s = s[:i]
		}
		var b strings.Builder
		for _, r := range s {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		return strings.TrimLeft(b.String(), "0")
	}
	n := normalize(nationalID)
	return n != "" && normalize(odooDocument) == n
}

// LivestockCreditInstallment is one installment of a quota invoice.
type LivestockCreditInstallment struct {
	Number   int             `json:"number"`
	DueDate  string          `json:"dueDate"`
	Amount   decimal.Decimal `json:"amount"`
	Residual decimal.Decimal `json:"residual"`
}

// LivestockCreditInvoice is a quota invoice of a credit line's Odoo contact,
// as Odoo last reported it. One row per Odoo invoice and line.
type LivestockCreditInvoice struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreditLineID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_livestock_credit_invoices_line_invoice"`
	OdooInvoiceID int64     `gorm:"not null;uniqueIndex:idx_livestock_credit_invoices_line_invoice"`

	Number   string `gorm:"type:varchar(40)"`
	OrderRef string `gorm:"type:varchar(40)"`
	// Dates as Odoo sends them, YYYY-MM-DD.
	InvoiceDate string `gorm:"type:varchar(10)"`
	DueDate     string `gorm:"type:varchar(10)"`

	AmountTotal    decimal.Decimal              `gorm:"type:decimal(18,2);not null"`
	AmountResidual decimal.Decimal              `gorm:"type:decimal(18,2);not null"`
	PaymentState   string                       `gorm:"type:varchar(20)"`
	Installments   []LivestockCreditInstallment `gorm:"serializer:json;type:text"`

	// LoanID is Prestia's loan of the invoice (capital and dates from Odoo,
	// interest from the credit line's rate); nil until opened, or when the
	// line has no credit line in Prestia.
	LoanID *uuid.UUID `gorm:"type:uuid;index"`

	SyncedAt  time.Time `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewLivestockCreditInvoice records an Odoo invoice for a line.
func NewLivestockCreditInvoice(lineID uuid.UUID, inv OdooInvoice, at time.Time) LivestockCreditInvoice {
	i := LivestockCreditInvoice{ID: uuid.New(), CreditLineID: lineID, OdooInvoiceID: inv.ID}
	i.Refresh(inv, at)
	return i
}

// Refresh takes the invoice as Odoo reports it now.
func (i *LivestockCreditInvoice) Refresh(inv OdooInvoice, at time.Time) {
	i.Number = truncate(inv.Number, 40)
	i.OrderRef = truncate(inv.Order, 40)
	i.InvoiceDate = truncate(inv.Date, 10)
	i.DueDate = truncate(inv.DueDate, 10)
	i.AmountTotal = inv.AmountTotal.Round(2)
	i.AmountResidual = inv.AmountResidual.Round(2)
	i.PaymentState = truncate(inv.PaymentState, 20)
	i.Installments = append([]LivestockCreditInstallment(nil), inv.Installments...)
	i.SyncedAt = at.UTC()
}

// IsOpen tells whether the invoice still has a balance in Odoo.
func (i LivestockCreditInvoice) IsOpen() bool {
	return i.AmountResidual.IsPositive()
}

// NextDueDate is the due date of the oldest installment with a balance, or
// the invoice's due date when no installment does.
func (i LivestockCreditInvoice) NextDueDate() string {
	next := ""
	for _, inst := range i.Installments {
		if inst.Residual.IsPositive() && (next == "" || inst.DueDate < next) {
			next = inst.DueDate
		}
	}
	if next == "" {
		return i.DueDate
	}
	return next
}

// SortInvoicesForCollection orders invoices for the payment form: open ones
// first, by the due date of their oldest unpaid installment (oldest first),
// then the paid ones, newest first.
func SortInvoicesForCollection(invoices []LivestockCreditInvoice) {
	sort.SliceStable(invoices, func(a, b int) bool {
		ia, ib := invoices[a], invoices[b]
		if ia.IsOpen() != ib.IsOpen() {
			return ia.IsOpen()
		}
		if ia.IsOpen() {
			if da, db := ia.NextDueDate(), ib.NextDueDate(); da != db {
				return da < db
			}
			return ia.Number < ib.Number
		}
		if ia.InvoiceDate != ib.InvoiceDate {
			return ia.InvoiceDate > ib.InvoiceDate
		}
		return ia.Number > ib.Number
	})
}

// OldestOpenInvoice is the invoice the payment form preselects: the open one
// whose oldest unpaid installment is due first. Nil when none is open.
func OldestOpenInvoice(invoices []LivestockCreditInvoice) *LivestockCreditInvoice {
	sorted := append([]LivestockCreditInvoice(nil), invoices...)
	SortInvoicesForCollection(sorted)
	if len(sorted) == 0 || !sorted[0].IsOpen() {
		return nil
	}
	return &sorted[0]
}

// LivestockCreditPaymentStatus is the outcome of a payment sent to Odoo.
type LivestockCreditPaymentStatus string

const (
	// PaymentRegistered: Odoo created the payment and reconciled it.
	PaymentRegistered LivestockCreditPaymentStatus = "registered"
	// PaymentAlreadyRegistered: Odoo already had a payment with that
	// reference (409 PAYMENT_ALREADY_REGISTERED), so nothing was duplicated.
	PaymentAlreadyRegistered LivestockCreditPaymentStatus = "already_registered"
	// PaymentFailed: Odoo rejected it (4xx) and created nothing.
	PaymentFailed LivestockCreditPaymentStatus = "failed"
)

// LivestockCreditPayment is a rancher's payment staff registered in Prestia and
// sent to Odoo, with Odoo's outcome. A reference (the bank receipt number) is
// used by at most one payment that is not failed.
type LivestockCreditPayment struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreditLineID  uuid.UUID `gorm:"type:uuid;not null;index"`
	OdooInvoiceID int64     `gorm:"not null"`
	InvoiceNumber string    `gorm:"type:varchar(40)"`

	Amount decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	// CapitalAmount and InterestAmount split Amount on an invoice with a
	// Prestia loan: only the capital is sent to Odoo; the interest stays in
	// Prestia (InterestReportedAt nil) until Odoo can take it. On a payment
	// without loan, Amount is all capital.
	CapitalAmount      decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	InterestAmount     decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	InterestReportedAt *time.Time
	// LoanPaymentID is the payment in Prestia's loan (payments table).
	LoanPaymentID *uuid.UUID `gorm:"type:uuid"`
	PaymentDate   string     `gorm:"type:varchar(10);not null"`
	Bank          string     `gorm:"type:varchar(40);not null"`
	Reference     string     `gorm:"type:varchar(100);not null;index;uniqueIndex:idx_livestock_credit_payments_active_reference,where:status <> 'failed'"`

	RegisteredBy      *uuid.UUID `gorm:"type:uuid"`
	RegisteredByEmail string     `gorm:"type:varchar(200)"`

	OdooPaymentID     *int64
	OdooPaymentNumber string                       `gorm:"type:varchar(60)"`
	Status            LivestockCreditPaymentStatus `gorm:"type:varchar(20);not null"`
	ErrorCode         string                       `gorm:"type:varchar(60)"`
	ErrorMessage      string                       `gorm:"type:varchar(500)"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// LivestockCreditPaymentRequest is a payment staff wants to register.
type LivestockCreditPaymentRequest struct {
	InvoiceID int64
	Amount    decimal.Decimal
	// Date is YYYY-MM-DD; empty means today.
	Date      string
	Bank      string
	Reference string
}

// MaxPaymentReferenceLength bounds the bank receipt number.
const MaxPaymentReferenceLength = 100

// Validate normalizes the request and checks it on its own: a positive amount
// with at most two decimals, a date that is not in the future (today, in the
// deployment's time zone, is passed in as YYYY-MM-DD), a bank and a
// reference.
func (r *LivestockCreditPaymentRequest) Validate(today string) error {
	r.Bank = strings.ToUpper(strings.TrimSpace(r.Bank))
	r.Reference = strings.TrimSpace(r.Reference)
	r.Date = strings.TrimSpace(r.Date)
	if r.Date == "" {
		r.Date = today
	}
	switch {
	case r.InvoiceID <= 0:
		return portfolioReject(PortfolioInvalid, "invoice_required", "choose the invoice the payment is for")
	case !r.Amount.IsPositive():
		return portfolioReject(PortfolioInvalid, "invalid_amount", "the amount must be greater than zero")
	case !r.Amount.Equal(r.Amount.Round(2)):
		return portfolioReject(PortfolioInvalid, "invalid_amount", "the amount can have at most two decimals")
	case r.Reference == "":
		return portfolioReject(PortfolioInvalid, "reference_required", "the payment reference (receipt number) is required")
	case len(r.Reference) > MaxPaymentReferenceLength:
		return portfolioReject(PortfolioInvalid, "invalid_reference", "the reference can have at most %d characters", MaxPaymentReferenceLength)
	case r.Bank == "":
		return portfolioReject(PortfolioInvalid, "bank_required", "choose the bank the payment was made into")
	}
	if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
		return portfolioReject(PortfolioInvalid, "invalid_date", "the date must be YYYY-MM-DD")
	}
	if r.Date > today {
		return portfolioReject(PortfolioInvalid, "future_date", "the payment date cannot be in the future")
	}
	r.Amount = r.Amount.Round(2)
	return nil
}

// NewLivestockCreditPayment records a payment request for a line's invoice,
// before its outcome is known.
func NewLivestockCreditPayment(lineID uuid.UUID, r LivestockCreditPaymentRequest, invoiceNumber string, by *uuid.UUID, byEmail string) *LivestockCreditPayment {
	return &LivestockCreditPayment{
		ID: uuid.New(), CreditLineID: lineID, OdooInvoiceID: r.InvoiceID,
		InvoiceNumber: truncate(invoiceNumber, 40), Amount: r.Amount, PaymentDate: r.Date,
		Bank: r.Bank, Reference: r.Reference, RegisteredBy: by, RegisteredByEmail: truncate(byEmail, 200),
	}
}

// MarkRegistered records the payment Odoo created.
func (p *LivestockCreditPayment) MarkRegistered(paymentID int64, paymentNumber string) {
	id := paymentID
	p.Status = PaymentRegistered
	p.OdooPaymentID = &id
	p.OdooPaymentNumber = truncate(paymentNumber, 60)
	p.ErrorCode, p.ErrorMessage = "", ""
}

var alreadyRegisteredPayment = regexp.MustCompile(`\(pago ([^)]+)\)`)

// MarkAlreadyRegistered records that Odoo already had a payment with the
// reference. Odoo names that payment in the message ("… (pago PBCOL/…)").
func (p *LivestockCreditPayment) MarkAlreadyRegistered(code, message string) {
	p.Status = PaymentAlreadyRegistered
	p.ErrorCode = truncate(code, 60)
	p.ErrorMessage = truncate(message, 500)
	if m := alreadyRegisteredPayment.FindStringSubmatch(message); m != nil {
		p.OdooPaymentNumber = truncate(m[1], 60)
	}
}

// MarkRecordedInPrestiaOnly is a payment with no capital (interest only):
// nothing goes to Odoo.
func (p *LivestockCreditPayment) MarkRecordedInPrestiaOnly() {
	p.Status = PaymentRegistered
	p.ErrorCode, p.ErrorMessage = "", ""
}

// MarkFailed records Odoo's rejection.
func (p *LivestockCreditPayment) MarkFailed(code, message string) {
	p.Status = PaymentFailed
	p.ErrorCode = truncate(code, 60)
	p.ErrorMessage = truncate(message, 500)
}

// Replays tells whether a request repeats this payment (same line, invoice,
// amount, bank and reference): a double submission, answered with this one.
func (p *LivestockCreditPayment) Replays(lineID uuid.UUID, r LivestockCreditPaymentRequest) bool {
	return p.CreditLineID == lineID && p.OdooInvoiceID == r.InvoiceID && p.Amount.Equal(r.Amount) &&
		p.Bank == r.Bank && p.Reference == r.Reference
}

// LivestockCreditBlockEvent is a block or unblock of a rancher's purchases on
// the quota decided in Prestia and sent to Odoo (credit.blocked).
type LivestockCreditBlockEvent struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	CreditLineID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Blocked      bool       `gorm:"not null"`
	Reason       string     `gorm:"type:varchar(500);not null"`
	UserID       *uuid.UUID `gorm:"type:uuid"`
	UserEmail    string     `gorm:"type:varchar(200)"`
	CreatedAt    time.Time
}

// MaxBlockReasonLength bounds the reason of a block or unblock.
const MaxBlockReasonLength = 500

// NewLivestockCreditBlockEvent validates and records a block decision.
func NewLivestockCreditBlockEvent(lineID uuid.UUID, blocked bool, reason string, by *uuid.UUID, byEmail string) (*LivestockCreditBlockEvent, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return nil, portfolioReject(PortfolioInvalid, "reason_required", "the reason is required")
	case len(reason) > MaxBlockReasonLength:
		return nil, portfolioReject(PortfolioInvalid, "invalid_reason", "the reason can have at most %d characters", MaxBlockReasonLength)
	}
	return &LivestockCreditBlockEvent{ID: uuid.New(), CreditLineID: lineID, Blocked: blocked, Reason: reason,
		UserID: by, UserEmail: truncate(byEmail, 200)}, nil
}

// PortfolioRejectionKind tells the HTTP layer which status a rejection gets.
type PortfolioRejectionKind int

const (
	// PortfolioInvalid is a request whose data does not hold (422).
	PortfolioInvalid PortfolioRejectionKind = iota
	// PortfolioConflict clashes with what is stored (409).
	PortfolioConflict
	// PortfolioOdooRejected is a request Odoo refused (422), with Odoo's code.
	PortfolioOdooRejected
	// PortfolioUnavailable is Odoo not answering, limiting or refusing
	// Prestia's credentials: nothing was stored and it can be retried (503).
	PortfolioUnavailable
	// PortfolioDisabled is the Odoo integration not configured (503).
	PortfolioDisabled
)

// PortfolioRejection is why a portfolio operation was not done. Code is
// stable; OdooCode is Odoo's own code when Odoo answered.
type PortfolioRejection struct {
	Kind     PortfolioRejectionKind
	Code     string
	Message  string
	OdooCode string
}

func (e *PortfolioRejection) Error() string { return e.Code + ": " + e.Message }

// Retryable tells whether the same request may succeed later as it is.
func (e *PortfolioRejection) Retryable() bool { return e.Kind == PortfolioUnavailable }

func portfolioReject(kind PortfolioRejectionKind, code, format string, args ...any) *PortfolioRejection {
	return &PortfolioRejection{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	// Cut on a rune boundary.
	for max > 0 && (s[max]&0xC0) == 0x80 {
		max--
	}
	return s[:max]
}
