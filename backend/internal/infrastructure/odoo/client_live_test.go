package odoo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/shopspring/decimal"
)

// TestLiveGuideScript runs section 4 of TVGAN's integration guide (steps 6 to
// 11) against Odoo's TEST environment with the fictitious customer DOS
// (document 1000000103, id 71817). It registers real payments there and
// blocks and unblocks the customer, so it is opt-in: set ODOO_LIVE_TEST=1,
// ODOO_API_BASE_URL and ODOO_API_TOKEN (never the production ones). Only
// ids, amounts and codes are printed.
func TestLiveGuideScript(t *testing.T) {
	if os.Getenv("ODOO_LIVE_TEST") != "1" {
		t.Skip("set ODOO_LIVE_TEST=1 with ODOO_API_BASE_URL and ODOO_API_TOKEN of the TEST environment")
	}
	c := New(Config{BaseURL: os.Getenv("ODOO_API_BASE_URL"), Token: os.Getenv("ODOO_API_TOKEN")})
	if c == nil {
		t.Fatal("ODOO_API_BASE_URL and ODOO_API_TOKEN are required")
	}
	ctx := context.Background()
	report := func(step string, err error) {
		var oe *port.OdooError
		if errors.As(err, &oe) {
			fmt.Printf("%s -> HTTP %d %s: %s\n", step, oe.Status, oe.Code, oe.Message)
		} else if err != nil {
			fmt.Printf("%s -> %v\n", step, err)
		}
	}
	credit := func(step string, cu *model.OdooCustomer) {
		fmt.Printf("%s -> id %d approved %s used %s available %s overdue_days %d blocked %v can_buy %v block_reason %q\n",
			step, cu.ID, cu.Credit.ApprovedAmount, cu.Credit.UsedAmount, cu.Credit.AvailableAmount,
			cu.Credit.OverdueDays, cu.Credit.Blocked, cu.Credit.CanBuy, cu.Credit.BlockReason)
	}

	found, err := c.SearchCustomers(ctx, "1000000103")
	report("search 1000000103", err)
	if err == nil {
		fmt.Printf("search 1000000103 -> %d result(s)\n", len(found))
		for _, f := range found {
			fmt.Printf("  id %d\n", f.ID)
		}
	}

	cu, err := c.GetCustomer(ctx, 71817)
	report("6 GET /customers/71817", err)
	if err == nil {
		credit("6 GET /customers/71817", cu)
	}

	invoices, complete, err := c.CustomerInvoices(ctx, 71817)
	report("7 GET /customers/71817/invoices?status=all", err)
	for _, inv := range invoices {
		fmt.Printf("7 invoice id %d %s due %s total %s residual %s state %s installments %d (complete list %v)\n",
			inv.ID, inv.Number, inv.DueDate, inv.AmountTotal, inv.AmountResidual, inv.PaymentState, len(inv.Installments), complete)
		for _, in := range inv.Installments {
			fmt.Printf("    #%d due %s amount %s residual %s\n", in.Number, in.DueDate, in.Amount, in.Residual)
		}
	}

	banks, err := c.Banks(ctx)
	report("GET /payments/banks", err)
	for _, b := range banks {
		fmt.Printf("bank %s (%s)\n", b.Bank, b.Journal)
	}

	stamp := time.Now().UTC().Format("20060102150405")
	pay := func(step string, invoiceID int64, amount, bank, ref string) {
		res, err := c.RegisterPayment(ctx, invoiceID, model.OdooPaymentRequest{
			Amount: decimal.RequireFromString(amount), Bank: bank, Reference: ref})
		report(step, err)
		if err != nil {
			return
		}
		fmt.Printf("%s -> payment %d %s\n", step, res.PaymentID, res.PaymentNumber)
		if res.Invoice != nil {
			fmt.Printf("    invoice %s residual %s state %s\n", res.Invoice.Number, res.Invoice.AmountResidual, res.Invoice.PaymentState)
			for _, in := range res.Invoice.Installments {
				fmt.Printf("    #%d residual %s\n", in.Number, in.Residual)
			}
		}
		if res.Customer != nil {
			credit("    customer", res.Customer)
		}
	}
	ref1, ref2 := "PRESTIA-TEST-"+stamp+"-1", "PRESTIA-TEST-"+stamp+"-2"
	pay("8 POST /invoices/27507/payments 100000 DAVIVIENDA "+ref1, 27507, "100000", "DAVIVIENDA", ref1)
	pay("9 POST /invoices/27506/payments 825260.8 BANCOLOMBIA "+ref2, 27506, "825260.8", "BANCOLOMBIA", ref2)
	pay("10 repeat 9 with "+ref2, 27506, "825260.8", "BANCOLOMBIA", ref2)

	cu, err = c.SetBlocked(ctx, 71817, true)
	report("11 PATCH credit blocked=true", err)
	if err == nil {
		credit("11 PATCH credit blocked=true", cu)
	}
	cu, err = c.SetBlocked(ctx, 71817, false)
	report("11 PATCH credit blocked=false", err)
	if err == nil {
		credit("11 PATCH credit blocked=false", cu)
	}
	fmt.Printf("request budget left this hour: %d\n", c.RemainingBudget())
}
