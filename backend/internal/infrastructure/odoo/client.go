// Package odoo is the client of TVGAN's Odoo API for CuppoGan
// (/api/v1/cuppogan): the ledger of the livestock credit, where the purchases
// on the quota are invoiced and the payments are collected.
package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/shopspring/decimal"
)

// Defaults keep Prestia well under Odoo's limit (60/min and 600/h per IP,
// shared with CuppoGan's service on the same egress IP), even with a few
// instances running.
const (
	DefaultTimeout           = 15 * time.Second
	DefaultRequestsPerMinute = 15
	DefaultRequestsPerHour   = 150
	// maxInvoicesPerCustomer is the largest page Odoo serves.
	maxInvoicesPerCustomer = 500
	banksTTL               = 10 * time.Minute
	maxResponseSize        = 4 << 20
)

// Config configures the client. With no token or no base URL there is no
// client: the portfolio features are off.
type Config struct {
	BaseURL           string
	Token             string
	Timeout           time.Duration
	RequestsPerMinute int
	RequestsPerHour   int
}

// Client calls Odoo with the bearer token, through the client-side throttle.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	limiter *limiter

	mu         sync.Mutex
	banks      []model.OdooBank
	banksUntil time.Time
}

var _ port.OdooPortfolioGateway = (*Client)(nil)

// New builds the client, or returns nil when it is not configured.
func New(cfg Config) *Client {
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = DefaultRequestsPerMinute
	}
	if cfg.RequestsPerHour <= 0 {
		cfg.RequestsPerHour = DefaultRequestsPerHour
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		token:   strings.TrimSpace(cfg.Token),
		http:    &http.Client{Timeout: cfg.Timeout},
		limiter: newLimiter(cfg.RequestsPerMinute, cfg.RequestsPerHour),
	}
}

func (c *Client) RemainingBudget() int { return c.limiter.Remaining() }

// Wire shapes, as Odoo sends them.

type credit struct {
	ValidationStatus   *string         `json:"validation_status"`
	Approved           bool            `json:"approved"`
	ApprovedAmount     decimal.Decimal `json:"approved_amount"`
	UsedAmount         decimal.Decimal `json:"used_amount"`
	AvailableAmount    decimal.Decimal `json:"available_amount"`
	PromissoryNoteDate *string         `json:"promissory_note_date"`
	Blocked            bool            `json:"blocked"`
	OverdueDays        int             `json:"overdue_days"`
	LastUpdate         *string         `json:"last_update"`
	CanBuy             bool            `json:"can_buy"`
	BlockReason        *string         `json:"block_reason"`
}

type customer struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Document *string `json:"document"`
	City     *string `json:"city"`
	State    *string `json:"state"`
	Credit   credit  `json:"credit"`
}

type installment struct {
	Number   int             `json:"number"`
	DueDate  string          `json:"due_date"`
	Amount   decimal.Decimal `json:"amount"`
	Residual decimal.Decimal `json:"residual"`
}

type invoice struct {
	ID             int64           `json:"id"`
	Number         string          `json:"number"`
	Date           string          `json:"date"`
	DueDate        string          `json:"due_date"`
	CustomerID     int64           `json:"customer_id"`
	Order          *string         `json:"order"`
	AmountTotal    decimal.Decimal `json:"amount_total"`
	AmountResidual decimal.Decimal `json:"amount_residual"`
	PaymentState   string          `json:"payment_state"`
	Installments   []installment   `json:"installments"`
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c customer) toModel() model.OdooCustomer {
	return model.OdooCustomer{
		ID: c.ID, Name: c.Name, Document: str(c.Document), City: str(c.City), State: str(c.State),
		Credit: model.OdooCredit{
			ValidationStatus: str(c.Credit.ValidationStatus), Approved: c.Credit.Approved,
			ApprovedAmount: c.Credit.ApprovedAmount, UsedAmount: c.Credit.UsedAmount,
			AvailableAmount: c.Credit.AvailableAmount, PromissoryNoteDate: str(c.Credit.PromissoryNoteDate),
			Blocked: c.Credit.Blocked, OverdueDays: c.Credit.OverdueDays, LastUpdate: str(c.Credit.LastUpdate),
			CanBuy: c.Credit.CanBuy, BlockReason: str(c.Credit.BlockReason),
		},
	}
}

func (i invoice) toModel() model.OdooInvoice {
	out := model.OdooInvoice{
		ID: i.ID, Number: i.Number, Date: i.Date, DueDate: i.DueDate, CustomerID: i.CustomerID,
		Order: str(i.Order), AmountTotal: i.AmountTotal, AmountResidual: i.AmountResidual,
		PaymentState: i.PaymentState,
	}
	for _, inst := range i.Installments {
		out.Installments = append(out.Installments, model.LivestockCreditInstallment{
			Number: inst.Number, DueDate: inst.DueDate, Amount: inst.Amount, Residual: inst.Residual,
		})
	}
	return out
}

func (c *Client) SearchCustomers(ctx context.Context, document string) ([]model.OdooCustomer, error) {
	var body struct {
		Count     int        `json:"count"`
		Customers []customer `json:"customers"`
	}
	if err := c.do(ctx, http.MethodGet, "/customers/search", url.Values{"document": {document}}, nil, &body); err != nil {
		return nil, err
	}
	out := make([]model.OdooCustomer, len(body.Customers))
	for i, cu := range body.Customers {
		out[i] = cu.toModel()
	}
	return out, nil
}

func (c *Client) GetCustomer(ctx context.Context, partnerID int64) (*model.OdooCustomer, error) {
	var body customer
	if err := c.do(ctx, http.MethodGet, "/customers/"+strconv.FormatInt(partnerID, 10), nil, nil, &body); err != nil {
		return nil, err
	}
	m := body.toModel()
	return &m, nil
}

func (c *Client) CustomerInvoices(ctx context.Context, partnerID int64) ([]model.OdooInvoice, bool, error) {
	var body struct {
		Count    int       `json:"count"`
		Invoices []invoice `json:"invoices"`
	}
	q := url.Values{"status": {"all"}, "limit": {strconv.Itoa(maxInvoicesPerCustomer)}}
	if err := c.do(ctx, http.MethodGet, "/customers/"+strconv.FormatInt(partnerID, 10)+"/invoices", q, nil, &body); err != nil {
		return nil, false, err
	}
	out := make([]model.OdooInvoice, len(body.Invoices))
	for i, inv := range body.Invoices {
		out[i] = inv.toModel()
	}
	return out, len(out) < maxInvoicesPerCustomer, nil
}

func (c *Client) GetInvoice(ctx context.Context, invoiceID int64) (*model.OdooInvoice, error) {
	var body invoice
	if err := c.do(ctx, http.MethodGet, "/invoices/"+strconv.FormatInt(invoiceID, 10), nil, nil, &body); err != nil {
		return nil, err
	}
	m := body.toModel()
	return &m, nil
}

// Banks are cached for a few minutes: they change rarely and every payment
// checks them.
func (c *Client) Banks(ctx context.Context) ([]model.OdooBank, error) {
	c.mu.Lock()
	if time.Now().Before(c.banksUntil) {
		banks := append([]model.OdooBank(nil), c.banks...)
		c.mu.Unlock()
		return banks, nil
	}
	c.mu.Unlock()

	var body struct {
		Banks []struct {
			Bank    string  `json:"bank"`
			Journal string  `json:"journal"`
			Account *string `json:"account"`
		} `json:"banks"`
	}
	if err := c.do(ctx, http.MethodGet, "/payments/banks", nil, nil, &body); err != nil {
		return nil, err
	}
	banks := make([]model.OdooBank, len(body.Banks))
	for i, b := range body.Banks {
		banks[i] = model.OdooBank{Bank: b.Bank, Journal: b.Journal, Account: str(b.Account)}
	}
	c.mu.Lock()
	c.banks, c.banksUntil = banks, time.Now().Add(banksTTL)
	c.mu.Unlock()
	return append([]model.OdooBank(nil), banks...), nil
}

func (c *Client) RegisterPayment(ctx context.Context, invoiceID int64, p model.OdooPaymentRequest) (*model.OdooPaymentResult, error) {
	req := struct {
		Amount    json.Number `json:"amount"`
		Date      string      `json:"date,omitempty"`
		Bank      string      `json:"bank"`
		Reference string      `json:"reference"`
	}{Amount: json.Number(p.Amount.String()), Date: p.Date, Bank: p.Bank, Reference: p.Reference}
	var body struct {
		PaymentID     int64     `json:"payment_id"`
		PaymentNumber string    `json:"payment_number"`
		Invoice       *invoice  `json:"invoice"`
		Customer      *customer `json:"customer"`
	}
	if err := c.do(ctx, http.MethodPost, "/invoices/"+strconv.FormatInt(invoiceID, 10)+"/payments", nil, req, &body); err != nil {
		return nil, err
	}
	out := &model.OdooPaymentResult{PaymentID: body.PaymentID, PaymentNumber: body.PaymentNumber}
	if body.Invoice != nil {
		inv := body.Invoice.toModel()
		out.Invoice = &inv
	}
	if body.Customer != nil {
		cu := body.Customer.toModel()
		out.Customer = &cu
	}
	return out, nil
}

func (c *Client) SetBlocked(ctx context.Context, partnerID int64, blocked bool) (*model.OdooCustomer, error) {
	req := struct {
		Blocked bool `json:"blocked"`
	}{Blocked: blocked}
	var body customer
	if err := c.do(ctx, http.MethodPatch, "/customers/"+strconv.FormatInt(partnerID, 10)+"/credit", nil, req, &body); err != nil {
		return nil, err
	}
	m := body.toModel()
	return &m, nil
}

func (c *Client) SetApprovedAmount(ctx context.Context, partnerID int64, amount decimal.Decimal) (*model.OdooCustomer, error) {
	req := struct {
		ApprovedAmount json.Number `json:"approved_amount"`
	}{ApprovedAmount: json.Number(amount.Round(2).String())}
	var body customer
	if err := c.do(ctx, http.MethodPatch, "/customers/"+strconv.FormatInt(partnerID, 10)+"/credit", nil, req, &body); err != nil {
		return nil, err
	}
	m := body.toModel()
	return &m, nil
}

// do sends one request through the throttle and decodes a 2xx into out.
// Anything else is a *port.OdooError. Errors never carry the URL: its query
// may hold a national ID.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return &port.OdooError{Code: "THROTTLED", Message: "gave up waiting for the Odoo request budget"}
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encoding the Odoo request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return &port.OdooError{Code: "INVALID_REQUEST", Message: "the Odoo request cannot be built"}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &port.OdooError{Code: "NETWORK_ERROR", Message: networkMessage(err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return &port.OdooError{Code: "NETWORK_ERROR", Message: "the Odoo response could not be read"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e errorBody
		_ = json.Unmarshal(raw, &e)
		if e.Code == "" {
			e.Code = "HTTP_" + strconv.Itoa(resp.StatusCode)
		}
		if e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		return &port.OdooError{Status: resp.StatusCode, Code: e.Code, Message: e.Error}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &port.OdooError{Status: resp.StatusCode, Code: "INVALID_RESPONSE", Message: "Odoo answered a body Prestia cannot read"}
	}
	return nil
}

func networkMessage(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		if uerr.Timeout() {
			return "Odoo did not answer in time"
		}
		if errors.Is(uerr.Err, context.Canceled) {
			return "the request to Odoo was canceled"
		}
		return "Odoo is unreachable: " + uerr.Err.Error()
	}
	return "Odoo is unreachable"
}
