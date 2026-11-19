package odoo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "test-token"

const customerJSON = `{"id": 71817, "name": "GANADERO PRUEBA DOS CUPPOGAN", "document": "1000000103",
 "email": null, "phone": null, "city": "FLORENCIA", "state": "Caquetá",
 "credit": {"validation_status": "aprobado", "approved": true, "approved_amount": 5000000.0,
  "used_amount": 2576030, "available_amount": 2423970.0, "promissory_note_date": "2026-11-12",
  "blocked": false, "overdue_days": 12, "ruv": null, "last_update": "2026-11-12 13:09:58",
  "can_buy": false, "block_reason": "tiene cuotas vencidas (12 días de mora)"}}`

const invoiceJSON = `{"id": 27506, "number": "FE12262", "date": "2026-11-12", "due_date": "2027-02-10",
 "customer_id": 71817, "customer_name": "GANADERO PRUEBA DOS CUPPOGAN", "order": null,
 "amount_total": 2476030.0, "amount_residual": 1650769.2, "payment_state": "partial", "cufe": null,
 "installments": [{"number": 1, "due_date": "2026-12-12", "amount": 825260.8, "residual": 0.0},
  {"number": 2, "due_date": "2027-01-11", "amount": 825260.8, "residual": 825260.8},
  {"number": 3, "due_date": "2027-02-10", "amount": 825508.4, "residual": 825508.4}]}`

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error": "Token inválido.", "code": "INVALID_TOKEN"}`)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL + "/api/v1/cuppogan/", Token: testToken, RequestsPerMinute: 1000, RequestsPerHour: 1000})
}

func TestNewNeedsTokenAndURL(t *testing.T) {
	assert.Nil(t, New(Config{BaseURL: "https://x"}))
	assert.Nil(t, New(Config{Token: "t"}))
	assert.NotNil(t, New(Config{BaseURL: "https://x", Token: "t"}))
}

func TestReadsCustomerAndInvoices(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/cuppogan/customers/search":
			assert.Equal(t, "1000000103", r.URL.Query().Get("document"))
			_, _ = io.WriteString(w, `{"count": 1, "customers": [`+customerJSON+`]}`)
		case "/api/v1/cuppogan/customers/71817":
			_, _ = io.WriteString(w, customerJSON)
		case "/api/v1/cuppogan/customers/71817/invoices":
			assert.Equal(t, "all", r.URL.Query().Get("status"))
			assert.Equal(t, "500", r.URL.Query().Get("limit"))
			_, _ = io.WriteString(w, `{"customer_id": 71817, "count": 1, "invoices": [`+invoiceJSON+`]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	ctx := context.Background()
	found, err := c.SearchCustomers(ctx, "1000000103")
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.EqualValues(t, 71817, found[0].ID)

	cu, err := c.GetCustomer(ctx, 71817)
	require.NoError(t, err)
	assert.Equal(t, "1000000103", cu.Document)
	assert.Equal(t, 12, cu.Credit.OverdueDays)
	assert.False(t, cu.Credit.CanBuy)
	assert.True(t, cu.Credit.UsedAmount.Equal(decimal.NewFromInt(2576030)))
	assert.Equal(t, "tiene cuotas vencidas (12 días de mora)", cu.Credit.BlockReason)

	invoices, complete, err := c.CustomerInvoices(ctx, 71817)
	require.NoError(t, err)
	assert.True(t, complete)
	require.Len(t, invoices, 1)
	assert.Equal(t, "", invoices[0].Order)
	require.Len(t, invoices[0].Installments, 3)
	assert.True(t, invoices[0].Installments[0].Residual.IsZero())
	assert.True(t, invoices[0].AmountResidual.Equal(decimal.RequireFromString("1650769.2")))
}

func TestRegisterPaymentSendsANumberAndDecodesTheResult(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/cuppogan/invoices/27506/payments", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		// The amount travels as a JSON number, not a string.
		assert.JSONEq(t, `{"amount": 825260.8, "date": "2026-11-16", "bank": "BANCOLOMBIA", "reference": "R-1"}`, string(raw))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"payment_id": 2112, "payment_number": "PBCOL/2026/01768", "invoice": `+invoiceJSON+`, "customer": `+customerJSON+`}`)
	})
	res, err := c.RegisterPayment(context.Background(), 27506, model.OdooPaymentRequest{
		Amount: decimal.RequireFromString("825260.80"), Date: "2026-11-16", Bank: "BANCOLOMBIA", Reference: "R-1"})
	require.NoError(t, err)
	assert.EqualValues(t, 2112, res.PaymentID)
	assert.Equal(t, "PBCOL/2026/01768", res.PaymentNumber)
	require.NotNil(t, res.Invoice)
	require.NotNil(t, res.Customer)
	assert.EqualValues(t, 71817, res.Customer.ID)
}

func TestSetBlockedSendsOnlyBlocked(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/v1/cuppogan/customers/71817/credit", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]any{"blocked": true}, body)
		_, _ = io.WriteString(w, strings.Replace(customerJSON, `"blocked": false`, `"blocked": true`, 1))
	})
	cu, err := c.SetBlocked(context.Background(), 71817, true)
	require.NoError(t, err)
	assert.True(t, cu.Credit.Blocked)
}

func TestSetApprovedAmountSendsOnlyTheAmountAsANumber(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/v1/cuppogan/customers/71817/credit", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"approved_amount": 1299155}`, string(raw))
		assert.NotContains(t, string(raw), `"1299155"`, "a number, not a string")
		_, _ = io.WriteString(w, customerJSON)
	})
	_, err := c.SetApprovedAmount(context.Background(), 71817, decimal.RequireFromString("1299155.00"))
	require.NoError(t, err)
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status      int
		body        string
		code        string
		unavailable bool
	}{
		{409, `{"error": "Ya se registró un pago con la referencia R-1 (pago PBCOL/2026/01768).", "code": "PAYMENT_ALREADY_REGISTERED"}`, "PAYMENT_ALREADY_REGISTERED", false},
		{422, `{"error": "bank debe ser: BANCOLOMBIA o DAVIVIENDA.", "code": "INVALID_BANK"}`, "INVALID_BANK", false},
		{404, `{"error": "No existe la factura a cupo.", "code": "INVOICE_NOT_FOUND"}`, "INVOICE_NOT_FOUND", false},
		{429, `{"error": "Límite de peticiones superado.", "code": "RATE_LIMIT_EXCEEDED"}`, "RATE_LIMIT_EXCEEDED", true},
		{403, `{"error": "IP no permitida.", "code": "IP_NOT_ALLOWED"}`, "IP_NOT_ALLOWED", true},
		{500, `{"error": "Error del servidor.", "code": "INTERNAL_ERROR"}`, "INTERNAL_ERROR", true},
		{502, `<html>bad gateway</html>`, "HTTP_502", true},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.GetInvoice(context.Background(), 1)
			var oe *port.OdooError
			require.True(t, errors.As(err, &oe), "%v", err)
			assert.Equal(t, tc.status, oe.Status)
			assert.Equal(t, tc.code, oe.Code)
			assert.Equal(t, tc.unavailable, oe.Unavailable())
		})
	}
}

func TestWrongTokenIsUnavailableNotARejection(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	c.token = "wrong"
	_, err := c.GetCustomer(context.Background(), 1)
	var oe *port.OdooError
	require.True(t, errors.As(err, &oe))
	assert.Equal(t, "INVALID_TOKEN", oe.Code)
	assert.True(t, oe.Unavailable())
}

func TestNetworkErrorsDoNotCarryTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New(Config{BaseURL: url, Token: testToken})
	_, err := c.SearchCustomers(context.Background(), "1000000103")
	var oe *port.OdooError
	require.True(t, errors.As(err, &oe))
	assert.Equal(t, 0, oe.Status)
	assert.True(t, oe.Unavailable())
	assert.NotContains(t, err.Error(), "1000000103")
	assert.NotContains(t, err.Error(), testToken)
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, Token: testToken, Timeout: 50 * time.Millisecond})
	_, err := c.GetCustomer(context.Background(), 1)
	var oe *port.OdooError
	require.True(t, errors.As(err, &oe))
	assert.Equal(t, "NETWORK_ERROR", oe.Code)
	assert.Contains(t, oe.Message, "in time")
}

func TestBanksAreCached(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"banks": [{"bank": "BANCOLOMBIA", "journal": "Bancolombia", "account": "03175520666"},
			{"bank": "DAVIVIENDA", "journal": "Davivienda", "account": null}]}`)
	})
	for i := 0; i < 3; i++ {
		banks, err := c.Banks(context.Background())
		require.NoError(t, err)
		require.Len(t, banks, 2)
		assert.Equal(t, "", banks[1].Account)
	}
	assert.EqualValues(t, 1, calls.Load())
}

func TestLimiterWindows(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	l := newLimiter(2, 3)
	l.now = func() time.Time { return now }
	assert.Equal(t, 3, l.Remaining())
	assert.Zero(t, l.reserve())
	assert.Zero(t, l.reserve())
	// The minute window is full: the third waits for the first to age out.
	assert.Equal(t, time.Minute, l.reserve())
	assert.Equal(t, 1, l.Remaining())
	now = now.Add(time.Minute)
	assert.Zero(t, l.reserve())
	assert.Equal(t, 0, l.Remaining())
	// The hour window is full now.
	now = now.Add(2 * time.Minute)
	assert.Equal(t, 57*time.Minute, l.reserve())
	now = now.Add(57 * time.Minute)
	assert.Zero(t, l.reserve())
}

func TestLimiterWaitHonoursTheContext(t *testing.T) {
	l := newLimiter(1, 0)
	require.NoError(t, l.Wait(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, l.Wait(ctx), context.DeadlineExceeded)
}
