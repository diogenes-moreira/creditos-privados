package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/auth"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/odoo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryPortfolio is the portfolio tables in memory, on top of the lines.
type memoryPortfolio struct {
	lines    *memoryCreditLines
	invoices map[uuid.UUID]map[int64]model.LivestockCreditInvoice
	payments []model.LivestockCreditPayment
	events   []model.LivestockCreditBlockEvent
}

func (m *memoryPortfolio) SaveOdooState(_ context.Context, line *model.LivestockCreditLine) error {
	for _, l := range m.lines.lines {
		if l.ID == line.ID {
			l.Odoo = line.Odoo
		}
	}
	return nil
}

func (m *memoryPortfolio) ListLinesForSync(context.Context, int) ([]model.LivestockCreditLine, error) {
	var out []model.LivestockCreditLine
	for _, l := range m.lines.lines {
		if l.Odoo.LinkStatus != model.OdooLinkAmbiguous {
			out = append(out, *l)
		}
	}
	return out, nil
}

func (m *memoryPortfolio) UpsertInvoices(_ context.Context, lineID uuid.UUID, invoices []model.LivestockCreditInvoice, complete bool) error {
	if m.invoices[lineID] == nil || complete {
		m.invoices[lineID] = map[int64]model.LivestockCreditInvoice{}
	}
	for _, inv := range invoices {
		m.invoices[lineID][inv.OdooInvoiceID] = inv
	}
	return nil
}

func (m *memoryPortfolio) ListInvoices(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditInvoice, error) {
	var out []model.LivestockCreditInvoice
	for _, inv := range m.invoices[lineID] {
		out = append(out, inv)
	}
	return out, nil
}

func (m *memoryPortfolio) CreatePayment(_ context.Context, p *model.LivestockCreditPayment) error {
	for _, e := range m.payments {
		if p.Status != model.PaymentFailed && e.Status != model.PaymentFailed && e.Reference == p.Reference {
			return errors.New("duplicate key")
		}
	}
	m.payments = append(m.payments, *p)
	return nil
}

func (m *memoryPortfolio) FindActivePaymentByReference(_ context.Context, ref string) (*model.LivestockCreditPayment, error) {
	for _, p := range m.payments {
		if p.Reference == ref && p.Status != model.PaymentFailed {
			return &p, nil
		}
	}
	return nil, nil
}

func (m *memoryPortfolio) ListPayments(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditPayment, error) {
	var out []model.LivestockCreditPayment
	for _, p := range m.payments {
		if p.CreditLineID == lineID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memoryPortfolio) CreateBlockEvent(_ context.Context, e *model.LivestockCreditBlockEvent) error {
	m.events = append(m.events, *e)
	return nil
}

func (m *memoryPortfolio) ListBlockEvents(_ context.Context, lineID uuid.UUID) ([]model.LivestockCreditBlockEvent, error) {
	var out []model.LivestockCreditBlockEvent
	for _, e := range m.events {
		if e.CreditLineID == lineID {
			out = append(out, e)
		}
	}
	return out, nil
}

// mockOdoo is an HTTP server shaped like TVGAN's Odoo API, with the test
// customer DOS (71817) and its two open invoices.
type mockOdoo struct {
	mu         sync.Mutex
	references map[string]bool
	blocked    bool
	// failPayments answers every payment with this status and body.
	failStatus int
	failBody   string
	requests   []string
}

const mockToken = "mock-token"

func (o *mockOdoo) customer() string {
	blocked, canBuy, reason := "false", "false", `"tiene cuotas vencidas (12 días de mora)"`
	if o.blocked {
		blocked, reason = "true", `"está bloqueado por CuppoGan"`
	}
	return `{"id": 71817, "name": "GANADERO PRUEBA DOS CUPPOGAN", "document": "1000000103", "city": "FLORENCIA", "state": "Caquetá",
	 "credit": {"validation_status": "aprobado", "approved": true, "approved_amount": 5000000.0, "used_amount": 2576030.0,
	 "available_amount": 2423970.0, "promissory_note_date": "2026-11-12", "blocked": ` + blocked + `, "overdue_days": 12,
	 "ruv": null, "last_update": "2026-11-12 13:09:58", "can_buy": ` + canBuy + `, "block_reason": ` + reason + `}}`
}

const mockInvoices = `[
 {"id": 27506, "number": "FE12262", "date": "2026-11-12", "due_date": "2027-02-10", "customer_id": 71817, "order": "COT02405",
  "amount_total": 2476030.0, "amount_residual": 2476030.0, "payment_state": "not_paid",
  "installments": [{"number": 1, "due_date": "2026-12-12", "amount": 825260.8, "residual": 825260.8},
   {"number": 2, "due_date": "2027-01-11", "amount": 825260.8, "residual": 825260.8},
   {"number": 3, "due_date": "2027-02-10", "amount": 825508.4, "residual": 825508.4}]},
 {"id": 27507, "number": "FE12263", "date": "2026-10-01", "due_date": "2026-11-04", "customer_id": 71817, "order": null,
  "amount_total": 100000.0, "amount_residual": 100000.0, "payment_state": "not_paid",
  "installments": [{"number": 1, "due_date": "2026-11-04", "amount": 100000.0, "residual": 100000.0}]}]`

func (o *mockOdoo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests = append(o.requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer "+mockToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error": "Token inválido.", "code": "INVALID_TOKEN"}`)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/cuppogan")
	switch {
	case path == "/customers/search":
		_, _ = io.WriteString(w, `{"count": 1, "customers": [`+o.customer()+`]}`)
	case path == "/customers/71817":
		_, _ = io.WriteString(w, o.customer())
	case path == "/customers/71817/invoices":
		_, _ = io.WriteString(w, `{"customer_id": 71817, "count": 2, "invoices": `+mockInvoices+`}`)
	case path == "/invoices/27507":
		var invoices []json.RawMessage
		_ = json.Unmarshal([]byte(mockInvoices), &invoices)
		_, _ = w.Write(invoices[1])
	case path == "/payments/banks":
		_, _ = io.WriteString(w, `{"banks": [{"bank": "BANCOLOMBIA", "journal": "Bancolombia", "account": "1"}, {"bank": "DAVIVIENDA", "journal": "Davivienda", "account": "2"}]}`)
	case path == "/invoices/27507/payments":
		if o.failStatus != 0 {
			w.WriteHeader(o.failStatus)
			_, _ = io.WriteString(w, o.failBody)
			return
		}
		var body struct{ Reference string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if o.references[body.Reference] {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": "Ya se registró un pago con la referencia `+body.Reference+` (pago PDAVI/2026/00001).", "code": "PAYMENT_ALREADY_REGISTERED"}`)
			return
		}
		o.references[body.Reference] = true
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"payment_id": 2200, "payment_number": "PDAVI/2026/00001",
		 "invoice": {"id": 27507, "number": "FE12263", "customer_id": 71817, "amount_total": 100000.0, "amount_residual": 0.0,
		  "payment_state": "paid", "installments": [{"number": 1, "due_date": "2026-11-04", "amount": 100000.0, "residual": 0.0}]},
		 "customer": `+strings.Replace(strings.Replace(o.customer(), `"overdue_days": 12`, `"overdue_days": 0`, 1), `"can_buy": false`, `"can_buy": true`, 1)+`}`)
	case path == "/customers/71817/credit":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		o.blocked, _ = body["blocked"].(bool)
		_, _ = io.WriteString(w, o.customer())
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error": "No existe.", "code": "INVOICE_NOT_FOUND"}`)
	}
}

type portfolioFixture struct {
	backOfficeFixture
	odoo      *mockOdoo
	portfolio *memoryPortfolio
	line      *model.LivestockCreditLine
}

func newPortfolioFixture(t *testing.T, withOdoo bool) portfolioFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mock := &mockOdoo{references: map[string]bool{}}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	authService := auth.NewLocalAuthService("test-secret", 1)
	users := &roleUsers{users: map[string]*model.User{}}
	for _, role := range []model.UserRole{model.RoleAdmin, model.RoleAdvisor, model.RoleClient, model.RoleVendor} {
		users.users[string(role)] = &model.User{ID: uuid.New(), FirebaseUID: string(role), Role: role, IsActive: true}
	}
	lines := &memoryCreditLines{}
	portfolio := &memoryPortfolio{lines: lines, invoices: map[uuid.UUID]map[int64]model.LivestockCreditInvoice{}}
	audit := &memoryAudit{}
	var client *odoo.Client
	if withOdoo {
		client = odoo.New(odoo.Config{BaseURL: srv.URL + "/api/v1/cuppogan", Token: mockToken, RequestsPerMinute: 1000, RequestsPerHour: 1000})
	}
	svc := service.NewLivestockCreditPortfolioService(lines, portfolio, nil)
	if client != nil {
		svc = service.NewLivestockCreditPortfolioService(lines, portfolio, client)
	}
	h := handler.NewLivestockCreditPortfolioHandler(svc.WithAudit(service.NewAuditService(audit)))

	r := gin.New()
	g := r.Group("/admin")
	g.Use(middleware.AuthMiddleware(authService), middleware.RequireRole(users, model.CreditPortfolioRoles()...))
	h.RegisterRoutes(g)

	line := &model.LivestockCreditLine{ID: uuid.New(), Status: model.LivestockCreditLineActive, NationalID: "1000000103"}
	lines.lines = append(lines.lines, line)
	return portfolioFixture{
		backOfficeFixture: backOfficeFixture{router: r, auth: authService, users: users, lines: lines, audit: audit},
		odoo:              mock, portfolio: portfolio, line: line,
	}
}

func (f portfolioFixture) path(suffix string) string {
	return "/admin/livestock-credit-lines/" + f.line.ID.String() + suffix
}

func decodePortfolio(t *testing.T, w *httptest.ResponseRecorder) dto.LivestockCreditPortfolioResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var p dto.LivestockCreditPortfolioResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	return p
}

func decodePortfolioError(t *testing.T, w *httptest.ResponseRecorder, status int) dto.PortfolioErrorResponse {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	var e dto.PortfolioErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e))
	return e
}

func TestPortfolioRoutesRoles(t *testing.T) {
	f := newPortfolioFixture(t, true)
	routes := []struct{ method, path string }{
		{http.MethodGet, f.path("/portfolio")},
		{http.MethodPost, f.path("/odoo/sync")},
		{http.MethodPut, f.path("/odoo/partner")},
		{http.MethodPost, f.path("/payments")},
		{http.MethodPost, f.path("/block")},
		{http.MethodGet, "/admin/livestock-credit-portfolio/banks"},
	}
	for _, rt := range routes {
		assert.Equal(t, http.StatusUnauthorized, f.do(t, "", rt.method, rt.path, map[string]any{}).Code, rt.path)
		for _, role := range []string{"client", "vendor"} {
			assert.Equal(t, http.StatusForbidden, f.do(t, role, rt.method, rt.path, map[string]any{}).Code, role+" "+rt.path)
		}
	}
	for _, role := range []string{"admin", "advisor"} {
		assert.Equal(t, http.StatusOK, f.do(t, role, http.MethodGet, f.path("/portfolio"), nil).Code, role)
		assert.Equal(t, http.StatusOK, f.do(t, role, http.MethodGet, "/admin/livestock-credit-portfolio/banks", nil).Code, role)
	}
	assert.Equal(t, http.StatusBadRequest, f.do(t, "admin", http.MethodGet, "/admin/livestock-credit-lines/nope/portfolio", nil).Code)
	assert.Equal(t, http.StatusNotFound, f.do(t, "admin", http.MethodGet, "/admin/livestock-credit-lines/"+uuid.NewString()+"/portfolio", nil).Code)
}

func TestPortfolioSyncPayAndBlockThroughOdoo(t *testing.T) {
	f := newPortfolioFixture(t, true)

	// Not linked yet: payments wait for the first sync.
	e := decodePortfolioError(t, f.do(t, "advisor", http.MethodPost, f.path("/payments"),
		map[string]any{"invoiceId": 27507, "amount": "100000", "bank": "DAVIVIENDA", "reference": "R-1"}), http.StatusConflict)
	assert.Equal(t, "odoo_not_linked", e.Code)

	p := decodePortfolio(t, f.do(t, "advisor", http.MethodPost, f.path("/odoo/sync"), nil))
	assert.True(t, p.Odoo.Enabled)
	assert.Equal(t, "linked", p.Odoo.LinkStatus)
	require.NotNil(t, p.Odoo.Credit)
	assert.Equal(t, 12, p.Odoo.Credit.OverdueDays)
	assert.Equal(t, "2423970.00", p.Odoo.Credit.AvailableAmount)
	require.Len(t, p.Invoices, 2)
	assert.Equal(t, "FE12263", p.Invoices[0].Number)
	require.NotNil(t, p.SuggestedInvoiceID)
	assert.EqualValues(t, 27507, *p.SuggestedInvoiceID)
	assert.Len(t, p.Invoices[1].Installments, 3)

	// Over the balance: rejected before reaching Odoo.
	e = decodePortfolioError(t, f.do(t, "advisor", http.MethodPost, f.path("/payments"),
		map[string]any{"invoiceId": 27507, "amount": "100000.50", "bank": "DAVIVIENDA", "reference": "R-1"}), http.StatusUnprocessableEntity)
	assert.Equal(t, "amount_exceeds_residual", e.Code)

	pay := map[string]any{"invoiceId": 27507, "amount": 100000, "bank": "DAVIVIENDA", "reference": "R-1"}
	w := f.do(t, "advisor", http.MethodPost, f.path("/payments"), pay)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var res dto.LivestockCreditPaymentResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, "registered", res.Payment.Status)
	assert.Equal(t, "PDAVI/2026/00001", res.Payment.OdooPaymentNumber)
	assert.Equal(t, "advisor@example.com", res.Payment.RegisteredBy)
	assert.Equal(t, 0, res.Portfolio.Odoo.Credit.OverdueDays)
	assert.True(t, res.Portfolio.Odoo.Credit.CanBuy)

	// Double submission: same answer, nothing sent.
	sent := len(f.odoo.requests)
	w = f.do(t, "advisor", http.MethodPost, f.path("/payments"), pay)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.True(t, res.Replayed)
	assert.Len(t, f.odoo.requests, sent)

	// Odoo knows a reference Prestia does not (e.g. a retry after a lost
	// answer): stored as already registered, not duplicated.
	f.odoo.references["R-2"] = true
	w = f.do(t, "advisor", http.MethodPost, f.path("/payments"),
		map[string]any{"invoiceId": 27507, "amount": 100000, "bank": "DAVIVIENDA", "reference": "R-2"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, "already_registered", res.Payment.Status)

	// Odoo's own rejection is stored as failed and explained.
	f.odoo.failStatus, f.odoo.failBody = 422, `{"error": "bank debe ser: BANCOLOMBIA o DAVIVIENDA.", "code": "INVALID_BANK"}`
	e = decodePortfolioError(t, f.do(t, "advisor", http.MethodPost, f.path("/payments"),
		map[string]any{"invoiceId": 27507, "amount": 10, "bank": "DAVIVIENDA", "reference": "R-3"}), http.StatusUnprocessableEntity)
	assert.Equal(t, "odoo_rejected", e.Code)
	assert.Equal(t, "INVALID_BANK", e.OdooCode)
	assert.False(t, e.Retryable)

	// Odoo down: 503, retryable, nothing stored.
	f.odoo.failStatus, f.odoo.failBody = 500, `{"error": "Error del servidor; no se guardó nada.", "code": "INTERNAL_ERROR"}`
	stored := len(f.portfolio.payments)
	w = f.do(t, "advisor", http.MethodPost, f.path("/payments"),
		map[string]any{"invoiceId": 27507, "amount": 10, "bank": "DAVIVIENDA", "reference": "R-4"})
	e = decodePortfolioError(t, w, http.StatusServiceUnavailable)
	assert.True(t, e.Retryable)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))
	assert.Len(t, f.portfolio.payments, stored)

	p = decodePortfolio(t, f.do(t, "advisor", http.MethodGet, f.path("/portfolio"), nil))
	assert.Len(t, p.Payments, 3)

	// Block needs a reason; then it reaches Odoo and the history.
	assert.Equal(t, http.StatusBadRequest, f.do(t, "admin", http.MethodPost, f.path("/block"), map[string]any{"reason": "x"}).Code)
	e = decodePortfolioError(t, f.do(t, "admin", http.MethodPost, f.path("/block"), map[string]any{"blocked": true, "reason": " "}), http.StatusUnprocessableEntity)
	assert.Equal(t, "reason_required", e.Code)
	p = decodePortfolio(t, f.do(t, "admin", http.MethodPost, f.path("/block"), map[string]any{"blocked": true, "reason": "Acuerdo incumplido"}))
	assert.True(t, p.Odoo.Credit.Blocked)
	require.Len(t, p.BlockEvents, 1)
	assert.Equal(t, "admin@example.com", p.BlockEvents[0].By)
	p = decodePortfolio(t, f.do(t, "admin", http.MethodPost, f.path("/block"), map[string]any{"blocked": false, "reason": "Pagó"}))
	assert.False(t, p.Odoo.Credit.Blocked)
	assert.Len(t, p.BlockEvents, 2)

	var actions []string
	for _, l := range f.audit.logs {
		actions = append(actions, l.Action)
	}
	assert.Contains(t, actions, "register_livestock_credit_payment")
	assert.Contains(t, actions, "block_livestock_credit_line")
	assert.Contains(t, actions, "unblock_livestock_credit_line")
}

func TestPortfolioWithoutOdooToken(t *testing.T) {
	f := newPortfolioFixture(t, false)
	p := decodePortfolio(t, f.do(t, "admin", http.MethodGet, f.path("/portfolio"), nil))
	assert.False(t, p.Odoo.Enabled)
	assert.Equal(t, "pending", p.Odoo.LinkStatus)
	e := decodePortfolioError(t, f.do(t, "admin", http.MethodPost, f.path("/odoo/sync"), nil), http.StatusServiceUnavailable)
	assert.Equal(t, "odoo_disabled", e.Code)
	e = decodePortfolioError(t, f.do(t, "admin", http.MethodGet, "/admin/livestock-credit-portfolio/banks", nil), http.StatusServiceUnavailable)
	assert.Equal(t, "odoo_disabled", e.Code)
}

func TestPortfolioWrongOdooTokenIsNotTheUsersFault(t *testing.T) {
	f := newPortfolioFixture(t, true)
	f.odoo.mu.Lock()
	f.odoo.references = map[string]bool{}
	f.odoo.mu.Unlock()
	// Swap the server's expected token by pointing a client with a bad one.
	gin.SetMode(gin.TestMode)
	srv := httptest.NewServer(f.odoo)
	t.Cleanup(srv.Close)
	client := odoo.New(odoo.Config{BaseURL: srv.URL + "/api/v1/cuppogan", Token: "wrong"})
	svc := service.NewLivestockCreditPortfolioService(f.lines, f.portfolio, client)
	r := gin.New()
	g := r.Group("/admin")
	g.Use(middleware.AuthMiddleware(f.auth), middleware.RequireRole(f.users, model.CreditPortfolioRoles()...))
	handler.NewLivestockCreditPortfolioHandler(svc).RegisterRoutes(g)
	f.router = r
	e := decodePortfolioError(t, f.do(t, "admin", http.MethodPost, f.path("/odoo/sync"), nil), http.StatusServiceUnavailable)
	assert.Equal(t, "odoo_auth_failed", e.Code)
	assert.Equal(t, "INVALID_TOKEN", e.OdooCode)
	assert.Equal(t, "INVALID_TOKEN", f.line.Odoo.LastSyncErrorCode)
}
