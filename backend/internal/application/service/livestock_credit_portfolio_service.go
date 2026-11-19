package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
)

// maxLinesPerSyncRound bounds one unattended round; the throttle's budget
// usually stops it earlier.
const maxLinesPerSyncRound = 500

// LivestockCreditPortfolio is the Odoo side of a credit line as Prestia last
// saw it: the link and credit snapshot (in the line), its invoices ordered for
// collection, the payments staff registered and the block history.
type LivestockCreditPortfolio struct {
	Enabled     bool
	Line        *model.LivestockCreditLine
	Invoices    []model.LivestockCreditInvoice
	Payments    []model.LivestockCreditPayment
	BlockEvents []model.LivestockCreditBlockEvent
	// Suggested is the invoice the payment form preselects: the open one
	// whose oldest unpaid installment is due first.
	Suggested *model.LivestockCreditInvoice
	// Loans are Prestia's loans of the invoices, by id; Today is the date
	// what is due is computed for.
	Loans map[uuid.UUID]*model.Loan
	Today time.Time
}

// PaymentOutcome is a payment registration's answer.
type PaymentOutcome struct {
	Payment *model.LivestockCreditPayment
	// Replayed is a double submission answered with the payment already
	// stored; nothing was sent to Odoo.
	Replayed  bool
	Portfolio *LivestockCreditPortfolio
}

// Actor is who does a portfolio operation.
type Actor struct {
	UserID *uuid.UUID
	Email  string
}

// LivestockCreditPortfolioService is the portfolio (cartera) of the livestock
// credit lines on TVGAN's Odoo: it links each line to its Odoo contact, keeps
// a snapshot of the credit and invoices Odoo computes, and sends the payments
// and blocks staff register. Odoo is the ledger: Prestia shows its balances
// and never computes its own.
type LivestockCreditPortfolioService struct {
	lines   port.LivestockCreditLineRepository
	repo    port.LivestockCreditPortfolioRepository
	odoo    port.OdooPortfolioGateway
	audit   *AuditService
	now     func() time.Time
	loc     *time.Location
	reserve int
	// creditLines and loans hold Prestia's side of the credit: each quota
	// invoice's loan (WithLoans). Without them invoices are only shown.
	creditLines port.CreditLineRepository
	loans       port.LoanRepository
}

// WithLoans makes Prestia keep the credit: every quota invoice of an enrolled
// line opens a loan (Odoo's capital and dates, the line's interest), and
// payments are applied to it, with only their capital sent to Odoo.
func (s *LivestockCreditPortfolioService) WithLoans(creditLines port.CreditLineRepository, loans port.LoanRepository) *LivestockCreditPortfolioService {
	s.creditLines, s.loans = creditLines, loans
	return s
}

func (s *LivestockCreditPortfolioService) keepsLoans() bool {
	return s.creditLines != nil && s.loans != nil
}

// NewLivestockCreditPortfolioService builds the service. A nil gateway (no
// Odoo token) leaves the stored data readable and every Odoo operation off.
func NewLivestockCreditPortfolioService(lines port.LivestockCreditLineRepository, repo port.LivestockCreditPortfolioRepository, odoo port.OdooPortfolioGateway) *LivestockCreditPortfolioService {
	return &LivestockCreditPortfolioService{lines: lines, repo: repo, odoo: odoo, now: time.Now, loc: time.UTC}
}

// WithAudit records payments, blocks and links in the audit log.
func (s *LivestockCreditPortfolioService) WithAudit(audit *AuditService) *LivestockCreditPortfolioService {
	s.audit = audit
	return s
}

// WithLocation sets the time zone "today" is taken in (payment dates).
func (s *LivestockCreditPortfolioService) WithLocation(loc *time.Location) *LivestockCreditPortfolioService {
	if loc != nil {
		s.loc = loc
	}
	return s
}

// WithClock replaces the clock (tests).
func (s *LivestockCreditPortfolioService) WithClock(now func() time.Time) *LivestockCreditPortfolioService {
	s.now = now
	return s
}

// WithSyncReserve is how many requests of the throttle's budget the
// unattended sync leaves for staff's own requests.
func (s *LivestockCreditPortfolioService) WithSyncReserve(n int) *LivestockCreditPortfolioService {
	s.reserve = n
	return s
}

// Enabled tells whether Odoo is configured.
func (s *LivestockCreditPortfolioService) Enabled() bool { return s.odoo != nil }

func (s *LivestockCreditPortfolioService) today() string {
	return s.now().In(s.loc).Format(time.DateOnly)
}

var errOdooDisabled = &model.PortfolioRejection{Kind: model.PortfolioDisabled, Code: "odoo_disabled",
	Message: "the Odoo integration is not configured (ODOO_API_TOKEN)"}

// Portfolio returns what Prestia stored for the line; it does not call Odoo.
func (s *LivestockCreditPortfolioService) Portfolio(ctx context.Context, lineID uuid.UUID) (*LivestockCreditPortfolio, error) {
	line, err := s.line(ctx, lineID)
	if err != nil {
		return nil, err
	}
	return s.portfolioOf(ctx, line)
}

func (s *LivestockCreditPortfolioService) portfolioOf(ctx context.Context, line *model.LivestockCreditLine) (*LivestockCreditPortfolio, error) {
	invoices, err := s.repo.ListInvoices(ctx, line.ID)
	if err != nil {
		return nil, fmt.Errorf("reading the invoices: %w", err)
	}
	model.SortInvoicesForCollection(invoices)
	payments, err := s.repo.ListPayments(ctx, line.ID)
	if err != nil {
		return nil, fmt.Errorf("reading the payments: %w", err)
	}
	events, err := s.repo.ListBlockEvents(ctx, line.ID)
	if err != nil {
		return nil, fmt.Errorf("reading the block history: %w", err)
	}
	loans := map[uuid.UUID]*model.Loan{}
	if s.keepsLoans() {
		for _, inv := range invoices {
			if inv.LoanID == nil {
				continue
			}
			loan, err := s.loans.FindByIDWithInstallments(ctx, *inv.LoanID)
			if err != nil {
				return nil, fmt.Errorf("reading the loan of invoice %s: %w", inv.Number, err)
			}
			loans[loan.ID] = loan
		}
	}
	today, _ := time.Parse(time.DateOnly, s.today())
	return &LivestockCreditPortfolio{Enabled: s.Enabled(), Line: line, Invoices: invoices, Payments: payments,
		BlockEvents: events, Suggested: model.OldestOpenInvoice(invoices), Loans: loans, Today: today}, nil
}

func (s *LivestockCreditPortfolioService) line(ctx context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	line, err := s.lines.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reading the credit line: %w", err)
	}
	if line == nil {
		return nil, ErrCreditLineNotFound
	}
	return line, nil
}

// linkedLine reads a line that must be linked to its Odoo contact.
func (s *LivestockCreditPortfolioService) linkedLine(ctx context.Context, id uuid.UUID) (*model.LivestockCreditLine, int64, error) {
	if !s.Enabled() {
		return nil, 0, errOdooDisabled
	}
	line, err := s.line(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	partnerID, ok := line.OdooPartner()
	if !ok {
		return nil, 0, &model.PortfolioRejection{Kind: model.PortfolioConflict, Code: "odoo_not_linked",
			Message: "the credit line is not linked to an Odoo contact yet: update it from Odoo first"}
	}
	return line, partnerID, nil
}

// Sync refreshes one line from Odoo now (the "update from Odoo" button). A
// line not linked yet is looked up by national ID, also when it was
// ambiguous before.
func (s *LivestockCreditPortfolioService) Sync(ctx context.Context, lineID uuid.UUID) (*LivestockCreditPortfolio, error) {
	if !s.Enabled() {
		return nil, errOdooDisabled
	}
	line, err := s.line(ctx, lineID)
	if err != nil {
		return nil, err
	}
	if err := s.syncLine(ctx, line, true); err != nil {
		return nil, err
	}
	return s.portfolioOf(ctx, line)
}

// SyncAll is one unattended round: active lines, least recently synced
// first, until the throttle's budget reaches the reserve or Odoo fails in a
// way the next line would also hit. Ambiguous lines wait for staff.
func (s *LivestockCreditPortfolioService) SyncAll(ctx context.Context) (synced int, err error) {
	if !s.Enabled() {
		return 0, errOdooDisabled
	}
	lines, err := s.repo.ListLinesForSync(ctx, maxLinesPerSyncRound)
	if err != nil {
		return 0, fmt.Errorf("listing lines to sync: %w", err)
	}
	for i := range lines {
		if s.odoo.RemainingBudget() <= s.reserve {
			log.Printf("Odoo sync: request budget at its reserve, %d of %d lines left for the next round", len(lines)-i, len(lines))
			break
		}
		if err := s.syncLine(ctx, &lines[i], false); err != nil {
			var r *model.PortfolioRejection
			if errors.As(err, &r) && r.Retryable() {
				log.Printf("Odoo sync: stopping the round at line %s: %s", lines[i].ID, r.Code)
				return synced, err
			}
			log.Printf("Odoo sync: line %s not synced: %v", lines[i].ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}

// Run syncs every interval until ctx ends. The first round waits a minute
// after start, so a new instance does not hit Odoo while it boots.
func (s *LivestockCreditPortfolioService) Run(ctx context.Context, interval time.Duration) {
	if !s.Enabled() || interval <= 0 {
		return
	}
	first := time.Minute
	if interval < first {
		first = interval
	}
	timer := time.NewTimer(first)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if n, err := s.SyncAll(ctx); err != nil {
			log.Printf("Odoo sync round ended early after %d lines", n)
		} else {
			log.Printf("Odoo sync round: %d lines synced", n)
		}
		timer.Reset(interval)
	}
}

// syncLine links the line if needed and refreshes its snapshot and invoices.
// Failures are kept on the line (last sync error) and returned as
// *model.PortfolioRejection.
func (s *LivestockCreditPortfolioService) syncLine(ctx context.Context, line *model.LivestockCreditLine, manual bool) error {
	partnerID, linked := line.OdooPartner()
	var customer *model.OdooCustomer
	if !linked {
		if line.Odoo.LinkStatusOrPending() == model.OdooLinkAmbiguous && !manual {
			return nil
		}
		found, err := s.odoo.SearchCustomers(ctx, line.NationalID)
		if err != nil {
			return s.syncFailed(ctx, line, err)
		}
		// Odoo's search already matches the document, with or without
		// verification digit: its answer is taken as is.
		matching := found
		switch len(matching) {
		case 0:
			line.MarkNotInOdoo(s.now())
			return s.saveState(ctx, line)
		case 1:
			customer = &matching[0]
			line.LinkOdooPartner(customer.ID)
			partnerID = customer.ID
			s.record(ctx, nil, "link_livestock_credit_line_odoo", line,
				fmt.Sprintf("credit line %s linked to Odoo contact %d by national ID", line.ID, partnerID))
		default:
			candidates := make([]model.OdooPartnerCandidate, len(matching))
			for i, c := range matching {
				candidates[i] = model.OdooPartnerCandidate{ID: c.ID, Name: c.Name, Document: c.Document, City: c.City, State: c.State}
			}
			line.MarkOdooAmbiguous(candidates, s.now())
			return s.saveState(ctx, line)
		}
	}
	if customer == nil {
		c, err := s.odoo.GetCustomer(ctx, partnerID)
		if err != nil {
			return s.syncFailed(ctx, line, err)
		}
		customer = c
	}
	return s.refresh(ctx, line, partnerID, *customer)
}

// refresh takes the contact's snapshot and reloads its invoices.
func (s *LivestockCreditPortfolioService) refresh(ctx context.Context, line *model.LivestockCreditLine, partnerID int64, customer model.OdooCustomer) error {
	invoices, complete, err := s.odoo.CustomerInvoices(ctx, partnerID)
	if err != nil {
		// Keep the snapshot we did get; the error says the invoices are stale.
		line.ApplyOdooCustomer(customer, s.now())
		return s.syncFailed(ctx, line, err)
	}
	now := s.now()
	rows := make([]model.LivestockCreditInvoice, len(invoices))
	for i, inv := range invoices {
		rows[i] = model.NewLivestockCreditInvoice(line.ID, inv, now)
	}
	if err := s.repo.UpsertInvoices(ctx, line.ID, rows, complete); err != nil {
		return fmt.Errorf("storing the invoices: %w", err)
	}
	s.openInvoiceLoans(ctx, line)
	customer = s.alignApprovedAmount(ctx, line, partnerID, customer)
	line.ApplyOdooCustomer(customer, now)
	return s.saveState(ctx, line)
}

// alignApprovedAmount keeps Odoo's approved quota equal to the rancher's
// credit line in Prestia (its max amount): if it differs (the line changed in
// Prestia, or Odoo was edited), Prestia sends its own with a PATCH and Odoo
// recomputes the available amount. Failures are logged and retried on the
// next sync. Returns the contact as Odoo now reports it.
func (s *LivestockCreditPortfolioService) alignApprovedAmount(ctx context.Context, line *model.LivestockCreditLine, partnerID int64, customer model.OdooCustomer) model.OdooCustomer {
	if !s.keepsLoans() || line.CreditLineID == nil || line.Status != model.LivestockCreditLineActive {
		return customer
	}
	creditLine, err := s.creditLines.FindByID(ctx, *line.CreditLineID)
	if err != nil {
		log.Printf("Odoo sync: line %s: credit line not read to align the quota: %v", line.ID, err)
		return customer
	}
	want := creditLine.MaxAmount.Round(2)
	if customer.Credit.ApprovedAmount.Round(2).Equal(want) {
		return customer
	}
	updated, err := s.odoo.SetApprovedAmount(ctx, partnerID, want)
	if err != nil {
		log.Printf("Odoo sync: line %s: approved quota not sent to Odoo: %v", line.ID, err)
		return customer
	}
	s.record(ctx, nil, "align_livestock_credit_quota_odoo", line,
		fmt.Sprintf("Odoo contact %d: approved quota %s → %s (Prestia's credit line)", partnerID,
			customer.Credit.ApprovedAmount.StringFixed(2), want.StringFixed(2)))
	return *updated
}

// openInvoiceLoans opens the loan of every quota invoice of the line that has
// none, on the line's credit line and rate. Failures are logged and retried on
// the next sync: they never stop the sync.
func (s *LivestockCreditPortfolioService) openInvoiceLoans(ctx context.Context, line *model.LivestockCreditLine) {
	if !s.keepsLoans() || line.CreditLineID == nil || line.ClientID == nil {
		return
	}
	invoices, err := s.repo.ListInvoices(ctx, line.ID)
	if err != nil {
		log.Printf("Odoo sync: line %s: invoices not read to open loans: %v", line.ID, err)
		return
	}
	var creditLine *model.CreditLine
	for _, inv := range invoices {
		if inv.LoanID != nil {
			continue
		}
		if creditLine == nil {
			if creditLine, err = s.creditLines.FindByID(ctx, *line.CreditLineID); err != nil {
				log.Printf("Odoo sync: line %s: credit line not read: %v", line.ID, err)
				return
			}
		}
		loan, err := model.NewLivestockInvoiceLoan(*line.ClientID, creditLine.ID, inv, creditLine.InterestRate)
		if err != nil {
			log.Printf("Odoo sync: line %s: no loan for invoice %s: %v", line.ID, inv.Number, err)
			continue
		}
		if err := s.repo.OpenInvoiceLoan(ctx, inv.ID, loan); err != nil {
			log.Printf("Odoo sync: line %s: loan of invoice %s not stored: %v", line.ID, inv.Number, err)
			continue
		}
		note := ""
		if inv.AmountResidual.LessThan(inv.AmountTotal) {
			// Paid in Odoo before Prestia kept the credit: those payments are
			// not in the loan. Staff reconciles it.
			note = fmt.Sprintf("; Odoo already shows %s paid, not applied to the loan",
				inv.AmountTotal.Sub(inv.AmountResidual).StringFixed(2))
		}
		s.record(ctx, nil, "open_livestock_invoice_loan", line,
			fmt.Sprintf("loan %s opened for invoice %s: %s in %d installments at %s %% nominal%s",
				loan.ID, inv.Number, loan.Principal.StringFixed(2), loan.NumInstallments, loan.InterestRate.StringFixed(4), note))
	}
}

func (s *LivestockCreditPortfolioService) saveState(ctx context.Context, line *model.LivestockCreditLine) error {
	if err := s.repo.SaveOdooState(ctx, line); err != nil {
		return fmt.Errorf("storing the Odoo state: %w", err)
	}
	return nil
}

// syncFailed keeps an Odoo failure on the line and returns it as a rejection.
func (s *LivestockCreditPortfolioService) syncFailed(ctx context.Context, line *model.LivestockCreditLine, err error) error {
	r := odooRejection(err)
	line.RecordOdooSyncFailure(r.OdooCode, r.Message, s.now())
	if saveErr := s.saveState(ctx, line); saveErr != nil {
		return saveErr
	}
	return r
}

// LinkPartner links a line to the Odoo contact staff picked (an ambiguous
// national ID). The contact's document must be the line's national ID.
func (s *LivestockCreditPortfolioService) LinkPartner(ctx context.Context, lineID uuid.UUID, partnerID int64, actor Actor) (*LivestockCreditPortfolio, error) {
	if !s.Enabled() {
		return nil, errOdooDisabled
	}
	if partnerID <= 0 {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invalid_partner", Message: "partnerId must be a positive Odoo contact id"}
	}
	line, err := s.line(ctx, lineID)
	if err != nil {
		return nil, err
	}
	if current, ok := line.OdooPartner(); ok && current != partnerID {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioConflict, Code: "already_linked",
			Message: fmt.Sprintf("the credit line is already linked to Odoo contact %d", current)}
	}
	customer, err := s.odoo.GetCustomer(ctx, partnerID)
	if err != nil {
		var oe *port.OdooError
		if errors.As(err, &oe) && oe.Code == port.OdooCustomerNotFound {
			return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "odoo_customer_not_found",
				Message: fmt.Sprintf("Odoo has no contact %d", partnerID), OdooCode: oe.Code}
		}
		return nil, odooRejection(err)
	}
	if !model.OdooDocumentMatches(customer.Document, line.NationalID) && !isCandidate(line, partnerID) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "document_mismatch",
			Message: fmt.Sprintf("Odoo contact %d has another document", partnerID)}
	}
	line.LinkOdooPartner(partnerID)
	s.record(ctx, actor.UserID, "link_livestock_credit_line_odoo", line,
		fmt.Sprintf("credit line %s linked to Odoo contact %d, picked by staff", line.ID, partnerID))
	if err := s.refresh(ctx, line, partnerID, *customer); err != nil {
		return nil, err
	}
	return s.portfolioOf(ctx, line)
}

// Banks are TVGAN's accounts Odoo accepts payments into.
func (s *LivestockCreditPortfolioService) Banks(ctx context.Context) ([]model.OdooBank, error) {
	if !s.Enabled() {
		return nil, errOdooDisabled
	}
	banks, err := s.odoo.Banks(ctx)
	if err != nil {
		return nil, odooRejection(err)
	}
	return banks, nil
}

// RegisterPayment validates a payment against a fresh read of the invoice and
// Odoo's banks, and sends it to Odoo:
//   - created: stored as registered, with Odoo's ids, and the snapshot is
//     updated from Odoo's answer;
//   - 409 PAYMENT_ALREADY_REGISTERED: stored as already_registered (nothing
//     duplicated) and the snapshot refreshed;
//   - another 4xx: stored as failed with Odoo's code, and rejected;
//   - Odoo unavailable (5xx, network, rate limit): nothing stored, the same
//     reference can be retried safely (Odoo deduplicates it).
//
// A reference already used by a payment that did not fail is rejected,
// except a double submission of that same payment, answered with it.
func (s *LivestockCreditPortfolioService) RegisterPayment(ctx context.Context, lineID uuid.UUID, req model.LivestockCreditPaymentRequest, actor Actor) (*PaymentOutcome, error) {
	line, partnerID, err := s.linkedLine(ctx, lineID)
	if err != nil {
		return nil, err
	}
	if err := req.Validate(s.today()); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindActivePaymentByReference(ctx, req.Reference)
	if err != nil {
		return nil, fmt.Errorf("looking up the reference: %w", err)
	}
	if existing != nil {
		return s.answerExisting(ctx, line, existing, req)
	}

	invoice, err := s.odoo.GetInvoice(ctx, req.InvoiceID)
	if err != nil {
		var oe *port.OdooError
		if errors.As(err, &oe) && oe.Code == port.OdooInvoiceNotFound {
			return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invoice_not_found",
				Message: "Odoo has no quota invoice with that id", OdooCode: oe.Code}
		}
		return nil, odooRejection(err)
	}
	if invoice.CustomerID != partnerID {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invoice_of_another_customer",
			Message: fmt.Sprintf("invoice %s is not this rancher's", invoice.Number)}
	}
	if s.keepsLoans() {
		row, err := s.invoiceRow(ctx, line.ID, invoice.ID)
		if err != nil {
			return nil, err
		}
		if row != nil && row.LoanID != nil {
			return s.registerLoanPayment(ctx, line, invoice, *row.LoanID, req, actor)
		}
	}
	if req.Amount.GreaterThan(invoice.AmountResidual) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "amount_exceeds_residual",
			Message: fmt.Sprintf("the amount (%s) exceeds the balance of invoice %s (%s)",
				req.Amount.StringFixed(2), invoice.Number, invoice.AmountResidual.StringFixed(2))}
	}
	banks, err := s.odoo.Banks(ctx)
	if err != nil {
		return nil, odooRejection(err)
	}
	if !bankAccepted(banks, req.Bank) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invalid_bank",
			Message: fmt.Sprintf("Odoo does not take payments into %s", req.Bank)}
	}

	payment := model.NewLivestockCreditPayment(line.ID, req, invoice.Number, actor.UserID, actor.Email)
	result, err := s.odoo.RegisterPayment(ctx, invoice.ID, model.OdooPaymentRequest{
		Amount: req.Amount, Date: req.Date, Bank: req.Bank, Reference: req.Reference,
	})
	var oe *port.OdooError
	switch {
	case err == nil:
		payment.MarkRegistered(result.PaymentID, result.PaymentNumber)
	case errors.As(err, &oe) && oe.Code == port.OdooPaymentAlreadyRegistered:
		payment.MarkAlreadyRegistered(oe.Code, oe.Message)
	case errors.As(err, &oe) && !oe.Unavailable():
		payment.MarkFailed(oe.Code, oe.Message)
	default:
		r := odooRejection(err)
		s.record(ctx, actor.UserID, "livestock_credit_payment_not_sent", line,
			fmt.Sprintf("payment %s of %s on invoice %s not registered, Odoo unavailable (%s); it can be retried",
				req.Reference, req.Amount.StringFixed(2), invoice.Number, r.OdooCode))
		return nil, r
	}

	if err := s.repo.CreatePayment(ctx, payment); err != nil {
		// A concurrent request with the same reference stored it first.
		if again, findErr := s.repo.FindActivePaymentByReference(ctx, req.Reference); findErr == nil && again != nil {
			return s.answerExisting(ctx, line, again, req)
		}
		return nil, fmt.Errorf("storing the payment: %w", err)
	}
	s.record(ctx, actor.UserID, "register_livestock_credit_payment", line,
		fmt.Sprintf("payment %s of %s on invoice %s into %s dated %s: %s %s %s",
			payment.Reference, payment.Amount.StringFixed(2), payment.InvoiceNumber, payment.Bank,
			payment.PaymentDate, payment.Status, payment.OdooPaymentNumber, payment.ErrorCode))

	switch payment.Status {
	case model.PaymentRegistered:
		s.applyPaymentResult(ctx, line, result)
	case model.PaymentAlreadyRegistered:
		// Best effort: the snapshot may be stale if Odoo fails now.
		_ = s.syncLine(ctx, line, false)
	case model.PaymentFailed:
		return nil, &model.PortfolioRejection{Kind: model.PortfolioOdooRejected, Code: "odoo_rejected",
			Message: oe.Message, OdooCode: oe.Code}
	}
	portfolio, err := s.portfolioOf(ctx, line)
	if err != nil {
		return nil, err
	}
	return &PaymentOutcome{Payment: payment, Portfolio: portfolio}, nil
}

func (s *LivestockCreditPortfolioService) invoiceRow(ctx context.Context, lineID uuid.UUID, odooInvoiceID int64) (*model.LivestockCreditInvoice, error) {
	invoices, err := s.repo.ListInvoices(ctx, lineID)
	if err != nil {
		return nil, fmt.Errorf("reading the invoices: %w", err)
	}
	for i := range invoices {
		if invoices[i].OdooInvoiceID == odooInvoiceID {
			return &invoices[i], nil
		}
	}
	return nil, nil
}

// registerLoanPayment registers what the rancher paid on an invoice with a
// Prestia loan. The invoice's price includes the interest, so the whole
// payment goes to Odoo, which applies it to the oldest installment; Prestia
// applies it the same way, splitting it into interest and capital, and frees
// the same amount of the quota. The loan is only touched once Odoo has the
// payment. A loan not stored after Odoo took it is fixed by retrying the same
// reference: Odoo answers it was already registered and the loan is updated.
func (s *LivestockCreditPortfolioService) registerLoanPayment(ctx context.Context, line *model.LivestockCreditLine, invoice *model.OdooInvoice, loanID uuid.UUID, req model.LivestockCreditPaymentRequest, actor Actor) (*PaymentOutcome, error) {
	loan, err := s.loans.FindByIDWithInstallments(ctx, loanID)
	if err != nil {
		return nil, fmt.Errorf("reading the loan of invoice %s: %w", invoice.Number, err)
	}
	if req.Amount.GreaterThan(invoice.AmountResidual) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "amount_exceeds_residual",
			Message: fmt.Sprintf("the amount (%s) exceeds the balance of invoice %s (%s)",
				req.Amount.StringFixed(2), invoice.Number, invoice.AmountResidual.StringFixed(2))}
	}
	on, err := time.Parse(time.DateOnly, req.Date)
	if err != nil {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invalid_date", Message: "the date must be YYYY-MM-DD"}
	}
	payoff := loan.LivestockPayoff(on)
	split, err := loan.ApplyLivestockPayment(req.Amount, on)
	if err != nil {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioConflict, Code: "odoo_balance_mismatch",
			Message: fmt.Sprintf("the amount (%s) exceeds what Prestia's loan of invoice %s still owes (%s): reconcile the invoice first",
				req.Amount.StringFixed(2), invoice.Number, payoff.StringFixed(2))}
	}
	banks, err := s.odoo.Banks(ctx)
	if err != nil {
		return nil, odooRejection(err)
	}
	if !bankAccepted(banks, req.Bank) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioInvalid, Code: "invalid_bank",
			Message: fmt.Sprintf("Odoo does not take payments into %s", req.Bank)}
	}

	payment := model.NewLivestockCreditPayment(line.ID, req, invoice.Number, actor.UserID, actor.Email)
	payment.CapitalAmount, payment.InterestAmount = split.Capital, split.Interest
	result, err := s.odoo.RegisterPayment(ctx, invoice.ID, model.OdooPaymentRequest{
		Amount: req.Amount, Date: req.Date, Bank: req.Bank, Reference: req.Reference,
	})
	var oe *port.OdooError
	switch {
	case err == nil:
		payment.MarkRegistered(result.PaymentID, result.PaymentNumber)
	case errors.As(err, &oe) && oe.Code == port.OdooPaymentAlreadyRegistered:
		payment.MarkAlreadyRegistered(oe.Code, oe.Message)
	case errors.As(err, &oe) && !oe.Unavailable():
		payment.MarkFailed(oe.Code, oe.Message)
		if storeErr := s.repo.CreatePayment(ctx, payment); storeErr != nil {
			return nil, fmt.Errorf("storing the rejected payment: %w", storeErr)
		}
		s.record(ctx, actor.UserID, "register_livestock_credit_payment", line,
			fmt.Sprintf("payment %s of %s on invoice %s rejected by Odoo: %s",
				req.Reference, req.Amount.StringFixed(2), invoice.Number, oe.Code))
		return nil, &model.PortfolioRejection{Kind: model.PortfolioOdooRejected, Code: "odoo_rejected", Message: oe.Message, OdooCode: oe.Code}
	default:
		r := odooRejection(err)
		s.record(ctx, actor.UserID, "livestock_credit_payment_not_sent", line,
			fmt.Sprintf("payment %s of %s on invoice %s not registered, Odoo unavailable (%s); it can be retried",
				req.Reference, req.Amount.StringFixed(2), invoice.Number, r.OdooCode))
		return nil, r
	}
	// The interest is in the invoice: Odoo has it with the payment.
	reported := s.now()
	payment.InterestReportedAt = &reported

	loanPayment, err := model.NewPayment(loan.ID, req.Amount, model.PaymentTransfer, req.Reference)
	if err != nil {
		return nil, err
	}
	payment.LoanPaymentID = &loanPayment.ID
	if err := s.repo.RecordLoanPayment(ctx, loan, loanPayment, req.Amount, payment); err != nil {
		if again, findErr := s.repo.FindActivePaymentByReference(ctx, req.Reference); findErr == nil && again != nil {
			return s.answerExisting(ctx, line, again, req)
		}
		s.record(ctx, actor.UserID, "livestock_credit_payment_not_stored", line,
			fmt.Sprintf("payment %s: Odoo has %s of invoice %s but the loan was not updated (%v); retry the same reference",
				req.Reference, req.Amount.StringFixed(2), invoice.Number, err))
		return nil, fmt.Errorf("storing the payment on the loan: %w", err)
	}
	s.record(ctx, actor.UserID, "register_livestock_credit_payment", line,
		fmt.Sprintf("payment %s of %s on invoice %s into %s dated %s: capital %s, interest %s (Odoo: %s %s)",
			payment.Reference, payment.Amount.StringFixed(2), payment.InvoiceNumber, payment.Bank, payment.PaymentDate,
			split.Capital.StringFixed(2), split.Interest.StringFixed(2), payment.Status, payment.OdooPaymentNumber))
	if result != nil {
		s.applyPaymentResult(ctx, line, result)
	} else {
		_ = s.syncLine(ctx, line, false)
	}
	portfolio, err := s.portfolioOf(ctx, line)
	if err != nil {
		return nil, err
	}
	return &PaymentOutcome{Payment: payment, Portfolio: portfolio}, nil
}

func (s *LivestockCreditPortfolioService) answerExisting(ctx context.Context, line *model.LivestockCreditLine, existing *model.LivestockCreditPayment, req model.LivestockCreditPaymentRequest) (*PaymentOutcome, error) {
	if !existing.Replays(line.ID, req) {
		return nil, &model.PortfolioRejection{Kind: model.PortfolioConflict, Code: "reference_already_used",
			Message: fmt.Sprintf("reference %s was already used for another payment (invoice %s, %s)",
				existing.Reference, existing.InvoiceNumber, existing.Amount.StringFixed(2))}
	}
	portfolio, err := s.portfolioOf(ctx, line)
	if err != nil {
		return nil, err
	}
	return &PaymentOutcome{Payment: existing, Replayed: true, Portfolio: portfolio}, nil
}

// applyPaymentResult updates the invoice and the credit snapshot from Odoo's
// answer to a payment, so no extra request is needed.
func (s *LivestockCreditPortfolioService) applyPaymentResult(ctx context.Context, line *model.LivestockCreditLine, result *model.OdooPaymentResult) {
	now := s.now()
	if result.Invoice != nil {
		row := model.NewLivestockCreditInvoice(line.ID, *result.Invoice, now)
		if err := s.repo.UpsertInvoices(ctx, line.ID, []model.LivestockCreditInvoice{row}, false); err != nil {
			log.Printf("Odoo payment on line %s: invoice not refreshed: %v", line.ID, err)
		}
	}
	if result.Customer != nil {
		line.ApplyOdooCustomer(*result.Customer, now)
		if err := s.saveState(ctx, line); err != nil {
			log.Printf("Odoo payment on line %s: snapshot not refreshed: %v", line.ID, err)
		}
	}
}

// SetBlocked blocks or unblocks the rancher's purchases on the quota in Odoo
// (credit.blocked only), with a mandatory reason kept in Prestia's history.
func (s *LivestockCreditPortfolioService) SetBlocked(ctx context.Context, lineID uuid.UUID, blocked bool, reason string, actor Actor) (*LivestockCreditPortfolio, error) {
	line, partnerID, err := s.linkedLine(ctx, lineID)
	if err != nil {
		return nil, err
	}
	event, err := model.NewLivestockCreditBlockEvent(line.ID, blocked, reason, actor.UserID, actor.Email)
	if err != nil {
		return nil, err
	}
	customer, err := s.odoo.SetBlocked(ctx, partnerID, blocked)
	if err != nil {
		return nil, odooRejection(err)
	}
	if err := s.repo.CreateBlockEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("storing the block history: %w", err)
	}
	action, verb := "unblock_livestock_credit_line", "unblocked"
	if blocked {
		action, verb = "block_livestock_credit_line", "blocked"
	}
	s.record(ctx, actor.UserID, action, line, fmt.Sprintf("Odoo contact %d %s: %s", partnerID, verb, event.Reason))
	line.ApplyOdooCustomer(*customer, s.now())
	if err := s.saveState(ctx, line); err != nil {
		return nil, err
	}
	return s.portfolioOf(ctx, line)
}

func (s *LivestockCreditPortfolioService) record(ctx context.Context, user *uuid.UUID, action string, line *model.LivestockCreditLine, description string) {
	if s.audit != nil {
		s.audit.Record(ctx, user, action, "livestock_credit_line", line.ID.String(), description)
	}
}

// isCandidate tells whether Odoo's search by the line's national ID returned
// the contact (an ambiguous line), which covers documents Odoo stores in
// another form (e.g. a NIT with its verification digit and no dash).
func isCandidate(line *model.LivestockCreditLine, partnerID int64) bool {
	for _, c := range line.Odoo.Candidates {
		if c.ID == partnerID {
			return true
		}
	}
	return false
}

func bankAccepted(banks []model.OdooBank, bank string) bool {
	for _, b := range banks {
		if b.Bank == bank {
			return true
		}
	}
	return false
}

// odooRejection maps an Odoo failure: unavailable ones (no answer, 5xx, rate
// limit, credentials or IP refused) can be retried; the rest are Odoo
// refusing the request.
func odooRejection(err error) *model.PortfolioRejection {
	var oe *port.OdooError
	if !errors.As(err, &oe) {
		return &model.PortfolioRejection{Kind: model.PortfolioUnavailable, Code: "odoo_unavailable",
			Message: "Odoo could not be reached", OdooCode: "UNKNOWN"}
	}
	if oe.Unavailable() {
		code := "odoo_unavailable"
		switch {
		case oe.Status == 429:
			code = "odoo_rate_limited"
		case oe.Status == 401 || oe.Status == 403:
			code = "odoo_auth_failed"
		}
		return &model.PortfolioRejection{Kind: model.PortfolioUnavailable, Code: code, Message: oe.Message, OdooCode: oe.Code}
	}
	return &model.PortfolioRejection{Kind: model.PortfolioOdooRejected, Code: "odoo_rejected", Message: oe.Message, OdooCode: oe.Code}
}

// AsPortfolioRejection unwraps a rejection, if err is one.
func AsPortfolioRejection(err error) (*model.PortfolioRejection, bool) {
	var r *model.PortfolioRejection
	ok := errors.As(err, &r)
	return r, ok
}
