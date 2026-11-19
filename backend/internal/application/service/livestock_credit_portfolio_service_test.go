package service_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOdoo plays TVGAN's Odoo.
type fakeOdoo struct {
	search    map[string][]model.OdooCustomer
	customers map[int64]model.OdooCustomer
	invoices  map[int64]model.OdooInvoice
	banks     []model.OdooBank
	// fail, when set, answers every call with it.
	fail error
	// pay answers RegisterPayment; nil applies the payment like Odoo does.
	pay      func(invoiceID int64, p model.OdooPaymentRequest) (*model.OdooPaymentResult, error)
	budget   int
	calls    map[string]int
	payments []model.OdooPaymentRequest
	blocked  []bool
	approved []decimal.Decimal
}

func newFakeOdoo() *fakeOdoo {
	dos := model.OdooCustomer{ID: 71817, Name: "GANADERO PRUEBA DOS CUPPOGAN", Document: "1000000103",
		Credit: model.OdooCredit{ValidationStatus: "aprobado", Approved: true, ApprovedAmount: dec("5000000"),
			UsedAmount: dec("2576030"), AvailableAmount: dec("2423970"), OverdueDays: 12,
			BlockReason: "tiene cuotas vencidas (12 días de mora)"}}
	return &fakeOdoo{
		search:    map[string][]model.OdooCustomer{"1000000103": {dos}},
		customers: map[int64]model.OdooCustomer{71817: dos},
		invoices: map[int64]model.OdooInvoice{
			27507: {ID: 27507, Number: "FE12263", Date: "2026-10-01", DueDate: "2026-11-04", CustomerID: 71817,
				AmountTotal: dec("100000"), AmountResidual: dec("100000"), PaymentState: "not_paid",
				Installments: []model.LivestockCreditInstallment{{Number: 1, DueDate: "2026-11-04", Amount: dec("100000"), Residual: dec("100000")}}},
			27506: {ID: 27506, Number: "FE12262", Date: "2026-11-12", DueDate: "2027-02-10", CustomerID: 71817,
				AmountTotal: dec("2476030"), AmountResidual: dec("2476030"), PaymentState: "not_paid",
				Installments: []model.LivestockCreditInstallment{
					{Number: 1, DueDate: "2026-12-12", Amount: dec("825260.8"), Residual: dec("825260.8")},
					{Number: 2, DueDate: "2027-01-11", Amount: dec("825260.8"), Residual: dec("825260.8")},
					{Number: 3, DueDate: "2027-02-10", Amount: dec("825508.4"), Residual: dec("825508.4")}}},
			99999: {ID: 99999, Number: "FE00001", CustomerID: 1, AmountResidual: dec("10")},
		},
		banks:  []model.OdooBank{{Bank: "BANCOLOMBIA"}, {Bank: "DAVIVIENDA"}},
		budget: 1000,
		calls:  map[string]int{},
	}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func (f *fakeOdoo) call(name string) error {
	f.calls[name]++
	f.budget--
	return f.fail
}

func (f *fakeOdoo) SearchCustomers(_ context.Context, document string) ([]model.OdooCustomer, error) {
	if err := f.call("search"); err != nil {
		return nil, err
	}
	return f.search[document], nil
}

func (f *fakeOdoo) GetCustomer(_ context.Context, id int64) (*model.OdooCustomer, error) {
	if err := f.call("customer"); err != nil {
		return nil, err
	}
	c, ok := f.customers[id]
	if !ok {
		return nil, &port.OdooError{Status: 404, Code: port.OdooCustomerNotFound, Message: "No existe el cliente."}
	}
	return &c, nil
}

func (f *fakeOdoo) CustomerInvoices(_ context.Context, id int64) ([]model.OdooInvoice, bool, error) {
	if err := f.call("invoices"); err != nil {
		return nil, false, err
	}
	var out []model.OdooInvoice
	for _, inv := range f.invoices {
		if inv.CustomerID == id {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, true, nil
}

func (f *fakeOdoo) GetInvoice(_ context.Context, id int64) (*model.OdooInvoice, error) {
	if err := f.call("invoice"); err != nil {
		return nil, err
	}
	inv, ok := f.invoices[id]
	if !ok {
		return nil, &port.OdooError{Status: 404, Code: port.OdooInvoiceNotFound, Message: "No existe la factura a cupo."}
	}
	return &inv, nil
}

func (f *fakeOdoo) Banks(context.Context) ([]model.OdooBank, error) {
	if err := f.call("banks"); err != nil {
		return nil, err
	}
	return f.banks, nil
}

func (f *fakeOdoo) RegisterPayment(_ context.Context, invoiceID int64, p model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
	f.calls["pay"]++
	f.payments = append(f.payments, p)
	if f.pay != nil {
		return f.pay(invoiceID, p)
	}
	// Like Odoo: oldest installment first, the credit freed at once.
	inv := f.invoices[invoiceID]
	left := p.Amount
	for i := range inv.Installments {
		take := decimal.Min(left, inv.Installments[i].Residual)
		inv.Installments[i].Residual = inv.Installments[i].Residual.Sub(take)
		left = left.Sub(take)
	}
	inv.AmountResidual = inv.AmountResidual.Sub(p.Amount)
	inv.PaymentState = "partial"
	f.invoices[invoiceID] = inv
	c := f.customers[inv.CustomerID]
	c.Credit.UsedAmount = c.Credit.UsedAmount.Sub(p.Amount)
	c.Credit.AvailableAmount = c.Credit.AvailableAmount.Add(p.Amount)
	if invoiceID == 27507 {
		c.Credit.OverdueDays, c.Credit.CanBuy, c.Credit.BlockReason = 0, true, ""
	}
	f.customers[inv.CustomerID] = c
	return &model.OdooPaymentResult{PaymentID: 2112, PaymentNumber: "PDAVI/2026/00001", Invoice: &inv, Customer: &c}, nil
}

func (f *fakeOdoo) SetBlocked(_ context.Context, id int64, blocked bool) (*model.OdooCustomer, error) {
	if err := f.call("block"); err != nil {
		return nil, err
	}
	f.blocked = append(f.blocked, blocked)
	c := f.customers[id]
	c.Credit.Blocked = blocked
	c.Credit.CanBuy = !blocked && c.Credit.OverdueDays == 0
	f.customers[id] = c
	return &c, nil
}

func (f *fakeOdoo) SetApprovedAmount(_ context.Context, id int64, amount decimal.Decimal) (*model.OdooCustomer, error) {
	if err := f.call("approved"); err != nil {
		return nil, err
	}
	c := f.customers[id]
	c.Credit.AvailableAmount = c.Credit.AvailableAmount.Add(amount.Sub(c.Credit.ApprovedAmount))
	c.Credit.ApprovedAmount = amount
	f.customers[id] = c
	f.approved = append(f.approved, amount)
	return &c, nil
}

func (f *fakeOdoo) RemainingBudget() int { return f.budget }

// portfolioStore is the lines and portfolio tables in memory; reads return
// copies, like a database.
type portfolioStore struct {
	port.LivestockCreditLineRepository
	port.LivestockCreditPortfolioRepository
	lines    map[uuid.UUID]model.LivestockCreditLine
	invoices map[uuid.UUID]map[int64]model.LivestockCreditInvoice
	payments []model.LivestockCreditPayment
	events   []model.LivestockCreditBlockEvent
}

func newPortfolioStore() *portfolioStore {
	return &portfolioStore{lines: map[uuid.UUID]model.LivestockCreditLine{}, invoices: map[uuid.UUID]map[int64]model.LivestockCreditInvoice{}}
}

func (s *portfolioStore) addLine(nationalID string) uuid.UUID {
	id := uuid.New()
	s.lines[id] = model.LivestockCreditLine{ID: id, Status: model.LivestockCreditLineActive, NationalID: nationalID,
		ActivatedAt: time.Now().Add(-time.Duration(len(s.lines)) * time.Hour)}
	return id
}

func (s *portfolioStore) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	l, ok := s.lines[id]
	if !ok {
		return nil, nil
	}
	return &l, nil
}

func (s *portfolioStore) SaveOdooState(_ context.Context, line *model.LivestockCreditLine) error {
	l := s.lines[line.ID]
	l.Odoo = line.Odoo
	s.lines[line.ID] = l
	return nil
}

func (s *portfolioStore) ListLinesForSync(_ context.Context, limit int) ([]model.LivestockCreditLine, error) {
	var out []model.LivestockCreditLine
	for _, l := range s.lines {
		if l.Status == model.LivestockCreditLineActive && l.Odoo.LinkStatus != model.OdooLinkAmbiguous {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		ta, tb := out[a].Odoo.LastSyncAt, out[b].Odoo.LastSyncAt
		if (ta == nil) != (tb == nil) {
			return ta == nil
		}
		if ta != nil && !ta.Equal(*tb) {
			return ta.Before(*tb)
		}
		return out[a].ActivatedAt.Before(out[b].ActivatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *portfolioStore) UpsertInvoices(_ context.Context, lineID uuid.UUID, invoices []model.LivestockCreditInvoice, complete bool) error {
	previous := s.invoices[lineID]
	if s.invoices[lineID] == nil || complete {
		keep := map[int64]model.LivestockCreditInvoice{}
		if !complete {
			keep = s.invoices[lineID]
		}
		if keep == nil {
			keep = map[int64]model.LivestockCreditInvoice{}
		}
		s.invoices[lineID] = keep
	}
	for _, inv := range invoices {
		// Like the Postgres upsert: the row keeps its id and loan.
		if prev, ok := previous[inv.OdooInvoiceID]; ok {
			inv.ID, inv.LoanID = prev.ID, prev.LoanID
		}
		s.invoices[lineID][inv.OdooInvoiceID] = inv
	}
	return nil
}

func (s *portfolioStore) ListInvoices(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditInvoice, error) {
	var out []model.LivestockCreditInvoice
	for _, inv := range s.invoices[lineID] {
		out = append(out, inv)
	}
	return out, nil
}

func (s *portfolioStore) CreatePayment(_ context.Context, p *model.LivestockCreditPayment) error {
	if p.Status != model.PaymentFailed {
		for _, e := range s.payments {
			if e.Reference == p.Reference && e.Status != model.PaymentFailed {
				return errors.New("duplicate key value violates unique constraint")
			}
		}
	}
	p.CreatedAt = time.Now()
	s.payments = append(s.payments, *p)
	return nil
}

func (s *portfolioStore) FindActivePaymentByReference(_ context.Context, ref string) (*model.LivestockCreditPayment, error) {
	for _, p := range s.payments {
		if p.Reference == ref && p.Status != model.PaymentFailed {
			return &p, nil
		}
	}
	return nil, nil
}

func (s *portfolioStore) ListPayments(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditPayment, error) {
	var out []model.LivestockCreditPayment
	for i := len(s.payments) - 1; i >= 0; i-- {
		if s.payments[i].CreditLineID == lineID {
			out = append(out, s.payments[i])
		}
	}
	return out, nil
}

func (s *portfolioStore) CreateBlockEvent(_ context.Context, e *model.LivestockCreditBlockEvent) error {
	s.events = append(s.events, *e)
	return nil
}

func (s *portfolioStore) ListBlockEvents(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditBlockEvent, error) {
	var out []model.LivestockCreditBlockEvent
	for _, e := range s.events {
		if e.CreditLineID == lineID {
			out = append(out, e)
		}
	}
	return out, nil
}

type auditEntries struct {
	port.AuditLogRepository
	logs []*model.AuditLog
}

func (a *auditEntries) Create(_ context.Context, l *model.AuditLog) error {
	a.logs = append(a.logs, l)
	return nil
}

func (a *auditEntries) actions() []string {
	var out []string
	for _, l := range a.logs {
		out = append(out, l.Action)
	}
	return out
}

var portfolioToday = time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)

func newPortfolioService(odoo *fakeOdoo) (*service.LivestockCreditPortfolioService, *portfolioStore, *auditEntries) {
	store := newPortfolioStore()
	audit := &auditEntries{}
	var gateway port.OdooPortfolioGateway
	if odoo != nil {
		gateway = odoo
	}
	s := service.NewLivestockCreditPortfolioService(store, store, gateway).
		WithAudit(service.NewAuditService(audit)).WithClock(func() time.Time { return portfolioToday })
	return s, store, audit
}

func rejection(t *testing.T, err error) *model.PortfolioRejection {
	t.Helper()
	r, ok := service.AsPortfolioRejection(err)
	require.True(t, ok, "expected a portfolio rejection, got %v", err)
	return r
}

var staff = service.Actor{UserID: func() *uuid.UUID { id := uuid.New(); return &id }(), Email: "cartera@example.com"}

func TestSyncLinksByNationalIDAndStoresInvoices(t *testing.T) {
	odoo := newFakeOdoo()
	s, store, audit := newPortfolioService(odoo)
	lineID := store.addLine("1000000103")

	p, err := s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	assert.Equal(t, model.OdooLinkLinked, p.Line.Odoo.LinkStatus)
	assert.EqualValues(t, 71817, *p.Line.Odoo.PartnerID)
	assert.Equal(t, 12, p.Line.Odoo.OverdueDays)
	assert.Equal(t, 1, odoo.calls["search"])
	assert.Zero(t, odoo.calls["customer"], "the search already brought the credit block")
	require.Len(t, p.Invoices, 2, "the other customer's invoice is not stored")
	assert.Equal(t, "FE12263", p.Invoices[0].Number, "oldest due first")
	require.NotNil(t, p.Suggested)
	assert.EqualValues(t, 27507, p.Suggested.OdooInvoiceID)
	assert.Contains(t, audit.actions(), "link_livestock_credit_line_odoo")

	// Linked: the next sync reads the contact by id.
	_, err = s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	assert.Equal(t, 1, odoo.calls["search"])
	assert.Equal(t, 1, odoo.calls["customer"])
	assert.Equal(t, 2, odoo.calls["invoices"])
}

func TestSyncWithoutMatchOrWithSeveral(t *testing.T) {
	odoo := newFakeOdoo()
	other := model.OdooCustomer{ID: 5000, Name: "DUPLICADO", Document: "1000000103"}
	odoo.search["1000000103"] = append(odoo.search["1000000103"], other)
	odoo.customers[5000] = model.OdooCustomer{ID: 5000, Document: "1000000103"}
	odoo.customers[6000] = model.OdooCustomer{ID: 6000, Document: "1000000999"}
	s, store, _ := newPortfolioService(odoo)
	missing := store.addLine("1000000555")
	ambiguous := store.addLine("1000000103")
	ctx := context.Background()

	p, err := s.Sync(ctx, missing)
	require.NoError(t, err)
	assert.Equal(t, model.OdooLinkNotFound, p.Line.Odoo.LinkStatus)

	p, err = s.Sync(ctx, ambiguous)
	require.NoError(t, err)
	assert.Equal(t, model.OdooLinkAmbiguous, p.Line.Odoo.LinkStatus)
	require.Len(t, p.Line.Odoo.Candidates, 2)

	// The unattended round leaves the ambiguous line for staff and searches
	// the missing one again.
	odoo.calls = map[string]int{}
	n, err := s.SyncAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, odoo.calls["search"])

	_, err = s.LinkPartner(ctx, ambiguous, 6000, staff)
	assert.Equal(t, "document_mismatch", rejection(t, err).Code)
	_, err = s.LinkPartner(ctx, ambiguous, 4242, staff)
	assert.Equal(t, "odoo_customer_not_found", rejection(t, err).Code)

	p, err = s.LinkPartner(ctx, ambiguous, 71817, staff)
	require.NoError(t, err)
	assert.Equal(t, model.OdooLinkLinked, p.Line.Odoo.LinkStatus)
	assert.Empty(t, p.Line.Odoo.Candidates)
	assert.Len(t, p.Invoices, 2)

	_, err = s.LinkPartner(ctx, ambiguous, 5000, staff)
	assert.Equal(t, "already_linked", rejection(t, err).Code)
}

func TestSyncFailureIsKeptOnTheLine(t *testing.T) {
	odoo := newFakeOdoo()
	s, store, _ := newPortfolioService(odoo)
	lineID := store.addLine("1000000103")
	_, err := s.Sync(context.Background(), lineID)
	require.NoError(t, err)

	odoo.fail = &port.OdooError{Code: "NETWORK_ERROR", Message: "Odoo did not answer in time"}
	_, err = s.Sync(context.Background(), lineID)
	r := rejection(t, err)
	assert.Equal(t, "odoo_unavailable", r.Code)
	assert.True(t, r.Retryable())

	stored := store.lines[lineID]
	assert.Equal(t, "NETWORK_ERROR", stored.Odoo.LastSyncErrorCode)
	assert.Equal(t, 12, stored.Odoo.OverdueDays, "the previous snapshot stays")
	assert.Len(t, store.invoices[lineID], 2)
}

func TestSyncAllStopsAtTheReserveAndWhenOdooIsDown(t *testing.T) {
	odoo := newFakeOdoo()
	s, store, _ := newPortfolioService(odoo)
	s.WithSyncReserve(10)
	for i := 0; i < 5; i++ {
		store.addLine("1000000103")
	}
	odoo.budget = 14 // two lines (search + invoices each) before the reserve
	n, err := s.SyncAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	odoo.budget = 1000
	odoo.fail = &port.OdooError{Status: 429, Code: "RATE_LIMIT_EXCEEDED", Message: "Límite de peticiones superado."}
	odoo.calls = map[string]int{}
	n, err = s.SyncAll(context.Background())
	assert.Equal(t, "odoo_rate_limited", rejection(t, err).Code)
	assert.Zero(t, n)
	assert.Equal(t, 1, odoo.calls["search"]+odoo.calls["customer"], "the round stops at the first line")
}

func TestPortfolioWithoutOdoo(t *testing.T) {
	s, store, _ := newPortfolioService(nil)
	lineID := store.addLine("1000000103")
	p, err := s.Portfolio(context.Background(), lineID)
	require.NoError(t, err)
	assert.False(t, p.Enabled)
	_, err = s.Sync(context.Background(), lineID)
	assert.Equal(t, "odoo_disabled", rejection(t, err).Code)
	_, err = s.RegisterPayment(context.Background(), lineID, model.LivestockCreditPaymentRequest{}, staff)
	assert.Equal(t, "odoo_disabled", rejection(t, err).Code)
	_, err = s.Portfolio(context.Background(), uuid.New())
	assert.ErrorIs(t, err, service.ErrCreditLineNotFound)
}

// linkedService has DOS's line linked and synced.
func linkedService(t *testing.T) (*service.LivestockCreditPortfolioService, *fakeOdoo, *portfolioStore, *auditEntries, uuid.UUID) {
	t.Helper()
	odoo := newFakeOdoo()
	s, store, audit := newPortfolioService(odoo)
	lineID := store.addLine("1000000103")
	_, err := s.Sync(context.Background(), lineID)
	require.NoError(t, err)
	odoo.calls = map[string]int{}
	return s, odoo, store, audit, lineID
}

func payReq(invoiceID int64, amount, bank, ref string) model.LivestockCreditPaymentRequest {
	return model.LivestockCreditPaymentRequest{InvoiceID: invoiceID, Amount: dec(amount), Bank: bank, Reference: ref}
}

func TestRegisterPayment(t *testing.T) {
	s, odoo, store, audit, lineID := linkedService(t)
	ctx := context.Background()

	out, err := s.RegisterPayment(ctx, lineID, payReq(27507, "100000", "davivienda", "PRESTIA-1"), staff)
	require.NoError(t, err)
	assert.False(t, out.Replayed)
	assert.Equal(t, model.PaymentRegistered, out.Payment.Status)
	assert.EqualValues(t, 2112, *out.Payment.OdooPaymentID)
	assert.Equal(t, "2026-11-16", out.Payment.PaymentDate, "today by default")
	assert.Equal(t, "cartera@example.com", out.Payment.RegisteredByEmail)
	require.Len(t, odoo.payments, 1)
	assert.Equal(t, "DAVIVIENDA", odoo.payments[0].Bank)
	// The snapshot comes from Odoo's answer, with no extra request.
	assert.Equal(t, 0, out.Portfolio.Line.Odoo.OverdueDays)
	assert.True(t, out.Portfolio.Line.Odoo.CanBuy)
	assert.Zero(t, odoo.calls["customer"])
	assert.True(t, store.invoices[lineID][27507].AmountResidual.IsZero())
	require.NotNil(t, out.Portfolio.Suggested)
	assert.EqualValues(t, 27506, out.Portfolio.Suggested.OdooInvoiceID, "the next open invoice")
	assert.Contains(t, audit.actions(), "register_livestock_credit_payment")

	// A double submission is answered with the stored payment.
	out, err = s.RegisterPayment(ctx, lineID, payReq(27507, "100000", "DAVIVIENDA", "PRESTIA-1"), staff)
	require.NoError(t, err)
	assert.True(t, out.Replayed)
	assert.Len(t, odoo.payments, 1)

	// The reference cannot be reused for another payment.
	_, err = s.RegisterPayment(ctx, lineID, payReq(27506, "5000", "DAVIVIENDA", "PRESTIA-1"), staff)
	r := rejection(t, err)
	assert.Equal(t, "reference_already_used", r.Code)
	assert.Equal(t, model.PortfolioConflict, r.Kind)
	assert.Len(t, odoo.payments, 1)
}

func TestRegisterPaymentValidatesBeforeSending(t *testing.T) {
	s, odoo, store, _, lineID := linkedService(t)
	ctx := context.Background()
	cases := map[string]model.LivestockCreditPaymentRequest{
		"amount_exceeds_residual":     payReq(27507, "100000.01", "DAVIVIENDA", "R-1"),
		"invalid_bank":                payReq(27507, "1000", "BANCO X", "R-2"),
		"invoice_of_another_customer": payReq(99999, "1", "DAVIVIENDA", "R-3"),
		"invoice_not_found":           payReq(123, "1", "DAVIVIENDA", "R-4"),
		"reference_required":          payReq(27507, "1", "DAVIVIENDA", " "),
		"invalid_amount":              payReq(27507, "-5", "DAVIVIENDA", "R-5"),
	}
	for code, req := range cases {
		_, err := s.RegisterPayment(ctx, lineID, req, staff)
		r := rejection(t, err)
		assert.Equal(t, code, r.Code)
		assert.Equal(t, model.PortfolioInvalid, r.Kind, code)
	}
	future := payReq(27507, "1", "DAVIVIENDA", "R-6")
	future.Date = "2026-11-17"
	_, err := s.RegisterPayment(ctx, lineID, future, staff)
	assert.Equal(t, "future_date", rejection(t, err).Code)

	assert.Empty(t, odoo.payments)
	assert.Empty(t, store.payments)

	unlinked := store.addLine("1000000999")
	_, err = s.RegisterPayment(ctx, unlinked, payReq(27507, "1", "DAVIVIENDA", "R-7"), staff)
	assert.Equal(t, "odoo_not_linked", rejection(t, err).Code)
}

func TestRegisterPaymentOdooOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("already registered in Odoo", func(t *testing.T) {
		s, odoo, store, _, lineID := linkedService(t)
		odoo.pay = func(int64, model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
			return nil, &port.OdooError{Status: 409, Code: "PAYMENT_ALREADY_REGISTERED",
				Message: "Ya se registró un pago con la referencia TRX-1 (pago PBCOL/2026/01768)."}
		}
		out, err := s.RegisterPayment(ctx, lineID, payReq(27506, "825260.8", "BANCOLOMBIA", "TRX-1"), staff)
		require.NoError(t, err)
		assert.Equal(t, model.PaymentAlreadyRegistered, out.Payment.Status)
		assert.Equal(t, "PBCOL/2026/01768", out.Payment.OdooPaymentNumber)
		assert.Len(t, store.payments, 1)
		assert.Equal(t, 1, odoo.calls["customer"], "the snapshot is refreshed")
	})

	t.Run("rejected by Odoo", func(t *testing.T) {
		s, odoo, store, _, lineID := linkedService(t)
		odoo.pay = func(int64, model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
			return nil, &port.OdooError{Status: 422, Code: "INVALID_DATE", Message: "La fecha no es válida."}
		}
		_, err := s.RegisterPayment(ctx, lineID, payReq(27506, "10", "BANCOLOMBIA", "TRX-2"), staff)
		r := rejection(t, err)
		assert.Equal(t, "odoo_rejected", r.Code)
		assert.Equal(t, "INVALID_DATE", r.OdooCode)
		require.Len(t, store.payments, 1)
		assert.Equal(t, model.PaymentFailed, store.payments[0].Status)
		assert.Equal(t, "INVALID_DATE", store.payments[0].ErrorCode)

		// A failed reference can be used again once fixed.
		odoo.pay = nil
		out, err := s.RegisterPayment(ctx, lineID, payReq(27506, "10", "BANCOLOMBIA", "TRX-2"), staff)
		require.NoError(t, err)
		assert.Equal(t, model.PaymentRegistered, out.Payment.Status)
	})

	t.Run("Odoo unavailable", func(t *testing.T) {
		s, odoo, store, audit, lineID := linkedService(t)
		odoo.pay = func(int64, model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
			return nil, &port.OdooError{Status: 500, Code: "INTERNAL_ERROR", Message: "Error del servidor; no se guardó nada."}
		}
		_, err := s.RegisterPayment(ctx, lineID, payReq(27506, "10", "BANCOLOMBIA", "TRX-3"), staff)
		r := rejection(t, err)
		assert.True(t, r.Retryable())
		assert.Empty(t, store.payments, "nothing stored")
		assert.Contains(t, audit.actions(), "livestock_credit_payment_not_sent")

		// The same reference is retried safely.
		odoo.pay = nil
		out, err := s.RegisterPayment(ctx, lineID, payReq(27506, "10", "BANCOLOMBIA", "TRX-3"), staff)
		require.NoError(t, err)
		assert.Equal(t, model.PaymentRegistered, out.Payment.Status)
	})
}

func TestSetBlocked(t *testing.T) {
	s, odoo, store, audit, lineID := linkedService(t)
	ctx := context.Background()

	_, err := s.SetBlocked(ctx, lineID, true, "  ", staff)
	assert.Equal(t, "reason_required", rejection(t, err).Code)
	assert.Empty(t, odoo.blocked)

	p, err := s.SetBlocked(ctx, lineID, true, "Acuerdo de pago incumplido", staff)
	require.NoError(t, err)
	assert.True(t, p.Line.Odoo.Blocked)
	assert.Equal(t, []bool{true}, odoo.blocked)
	require.Len(t, p.BlockEvents, 1)
	assert.Equal(t, "cartera@example.com", p.BlockEvents[0].UserEmail)

	odoo.fail = &port.OdooError{Status: 503, Code: "HTTP_503", Message: "Service Unavailable"}
	_, err = s.SetBlocked(ctx, lineID, false, "Pagó", staff)
	assert.True(t, rejection(t, err).Retryable())
	assert.Len(t, store.events, 1, "no history without Odoo's confirmation")

	odoo.fail = nil
	p, err = s.SetBlocked(ctx, lineID, false, "Pagó", staff)
	require.NoError(t, err)
	assert.False(t, p.Line.Odoo.Blocked)
	assert.Len(t, p.BlockEvents, 2)
	assert.Contains(t, audit.actions(), "block_livestock_credit_line")
	assert.Contains(t, audit.actions(), "unblock_livestock_credit_line")
}
