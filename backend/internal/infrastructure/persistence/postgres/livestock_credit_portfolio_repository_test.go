package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// legacyLine is a credit line row as stored before the Odoo columns existed.
type legacyLine struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Status               string
	AssessmentID         uuid.UUID `gorm:"type:uuid"`
	NationalID           string
	FullName             string
	Amount               decimal.Decimal `gorm:"type:decimal(18,2)"`
	Currency             string
	Partner              string
	PartnerReference     string
	PromissoryNoteNumber string
	SignedAt             time.Time
	ActivatedAt          time.Time
	DocumentSHA256       string `gorm:"column:document_sha256"`
	DocumentSize         int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (legacyLine) TableName() string { return "livestock_credit_lines" }

// TestPortfolioRepositoryOnPostgres runs against a real, disposable Postgres:
// set PRESTIA_TEST_DATABASE_DSN (e.g. a docker postgres:16). It drops and
// recreates the tables it uses.
func TestPortfolioRepositoryOnPostgres(t *testing.T) {
	dsn := os.Getenv("PRESTIA_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set PRESTIA_TEST_DATABASE_DSN to a disposable Postgres to run")
	}
	db, err := gorm.Open(pg.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	require.NoError(t, err)
	ctx := context.Background()
	m := db.Migrator()
	for _, table := range []any{&model.LivestockCreditBlockEvent{}, &model.LivestockCreditPayment{},
		&model.LivestockCreditInvoice{}, &model.LivestockCreditLineDocument{}, &model.LivestockCreditLine{}, &model.LivestockQuotaAssessment{}} {
		require.NoError(t, m.DropTable(table))
	}

	// A line stored before this change, then the migration adds the columns.
	require.NoError(t, db.AutoMigrate(&model.LivestockQuotaAssessment{}, &legacyLine{}))
	assessment := &model.LivestockQuotaAssessment{ID: uuid.New(), NationalID: "1000000103", Quota: decimal.NewFromInt(1000000), Currency: "COP"}
	require.NoError(t, db.Create(assessment).Error)
	old := legacyLine{ID: uuid.New(), Status: "active", AssessmentID: assessment.ID, NationalID: "1000000103", FullName: "GANADERO",
		Amount: decimal.NewFromInt(1000000), Currency: "COP", Partner: "cuppogan", PartnerReference: "ref-1",
		PromissoryNoteNumber: "CPG-2026-000009", SignedAt: time.Now(), ActivatedAt: time.Now(),
		DocumentSHA256: "00", DocumentSize: 1}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.AutoMigrate(&model.LivestockCreditLine{}, &model.LivestockCreditLineDocument{},
		&model.LivestockCreditInvoice{}, &model.LivestockCreditPayment{}, &model.LivestockCreditBlockEvent{}))

	lines := NewLivestockCreditLineRepository(db)
	repo := NewLivestockCreditPortfolioRepository(db)
	line, err := lines.FindByID(ctx, old.ID)
	require.NoError(t, err)
	require.NotNil(t, line)
	assert.Equal(t, model.OdooLinkPending, line.Odoo.LinkStatusOrPending())
	assert.Nil(t, line.Odoo.SnapshotAt)

	toSync, err := repo.ListLinesForSync(ctx, 10)
	require.NoError(t, err)
	require.Len(t, toSync, 1)

	// Ambiguous lines are left for staff.
	line.MarkOdooAmbiguous([]model.OdooPartnerCandidate{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, time.Now())
	require.NoError(t, repo.SaveOdooState(ctx, line))
	toSync, err = repo.ListLinesForSync(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, toSync)
	reread, _ := lines.FindByID(ctx, line.ID)
	assert.Len(t, reread.Odoo.Candidates, 2)

	// Link and snapshot; only the Odoo columns are written.
	line.LinkOdooPartner(71817)
	line.ApplyOdooCustomer(model.OdooCustomer{ID: 71817, Credit: model.OdooCredit{
		ApprovedAmount: decimal.NewFromInt(5000000), UsedAmount: decimal.RequireFromString("2576030"),
		AvailableAmount: decimal.RequireFromString("2423970"), OverdueDays: 12, CanBuy: false,
		BlockReason: "tiene cuotas vencidas (12 días de mora)"}}, time.Now())
	line.FullName = "NOT WRITTEN"
	require.NoError(t, repo.SaveOdooState(ctx, line))
	reread, _ = lines.FindByID(ctx, line.ID)
	assert.Equal(t, "GANADERO", reread.FullName)
	require.NotNil(t, reread.Odoo.PartnerID)
	assert.EqualValues(t, 71817, *reread.Odoo.PartnerID)
	assert.Empty(t, reread.Odoo.Candidates)
	assert.Equal(t, 12, reread.Odoo.OverdueDays)
	assert.True(t, reread.Odoo.AvailableAmount.Equal(decimal.RequireFromString("2423970")))

	// Invoices upsert by Odoo id; a complete list removes the missing ones.
	now := time.Now()
	inv := func(id int64, residual string) model.LivestockCreditInvoice {
		return model.NewLivestockCreditInvoice(line.ID, model.OdooInvoice{ID: id, Number: "FE" + decimal.NewFromInt(id).String(),
			DueDate: "2027-01-01", AmountTotal: decimal.NewFromInt(100), AmountResidual: decimal.RequireFromString(residual),
			Installments: []model.LivestockCreditInstallment{{Number: 1, DueDate: "2026-12-01", Amount: decimal.NewFromInt(100), Residual: decimal.RequireFromString(residual)}}}, now)
	}
	require.NoError(t, repo.UpsertInvoices(ctx, line.ID, []model.LivestockCreditInvoice{inv(1, "100"), inv(2, "100")}, true))
	require.NoError(t, repo.UpsertInvoices(ctx, line.ID, []model.LivestockCreditInvoice{inv(1, "40")}, false))
	got, err := repo.ListInvoices(ctx, line.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, g := range got {
		if g.OdooInvoiceID == 1 {
			assert.True(t, g.AmountResidual.Equal(decimal.NewFromInt(40)))
			assert.True(t, g.Installments[0].Residual.Equal(decimal.NewFromInt(40)))
		}
	}
	require.NoError(t, repo.UpsertInvoices(ctx, line.ID, []model.LivestockCreditInvoice{inv(2, "0")}, true))
	got, _ = repo.ListInvoices(ctx, line.ID)
	require.Len(t, got, 1)
	assert.EqualValues(t, 2, got[0].OdooInvoiceID)

	// A reference is used by at most one payment that did not fail.
	pay := func(status model.LivestockCreditPaymentStatus) *model.LivestockCreditPayment {
		p := model.NewLivestockCreditPayment(line.ID, model.LivestockCreditPaymentRequest{InvoiceID: 2,
			Amount: decimal.NewFromInt(10), Date: "2026-11-16", Bank: "BANCOLOMBIA", Reference: "REF-1"}, "FE2", nil, "")
		p.Status = status
		return p
	}
	require.NoError(t, repo.CreatePayment(ctx, pay(model.PaymentFailed)))
	require.NoError(t, repo.CreatePayment(ctx, pay(model.PaymentFailed)))
	found, err := repo.FindActivePaymentByReference(ctx, "REF-1")
	require.NoError(t, err)
	assert.Nil(t, found)
	require.NoError(t, repo.CreatePayment(ctx, pay(model.PaymentRegistered)))
	assert.Error(t, repo.CreatePayment(ctx, pay(model.PaymentAlreadyRegistered)))
	found, err = repo.FindActivePaymentByReference(ctx, "REF-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, model.PaymentRegistered, found.Status)
	payments, err := repo.ListPayments(ctx, line.ID)
	require.NoError(t, err)
	assert.Len(t, payments, 3)

	event, err := model.NewLivestockCreditBlockEvent(line.ID, true, "mora", nil, "")
	require.NoError(t, err)
	require.NoError(t, repo.CreateBlockEvent(ctx, event))
	events, err := repo.ListBlockEvents(ctx, line.ID)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
