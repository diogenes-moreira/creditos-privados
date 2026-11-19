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

// TestLivestockClientEnrollmentOnPostgres, like the portfolio test, needs
// PRESTIA_TEST_DATABASE_DSN pointing at a disposable Postgres.
func TestLivestockClientEnrollmentOnPostgres(t *testing.T) {
	dsn := os.Getenv("PRESTIA_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set PRESTIA_TEST_DATABASE_DSN to a disposable Postgres to run")
	}
	db, err := gorm.Open(pg.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	require.NoError(t, err)
	ctx := context.Background()
	tables := []any{&model.LivestockCreditLineDocument{}, &model.LivestockCreditLine{}, &model.LivestockQuotaAssessment{},
		&model.CreditLine{}, &model.CurrentAccount{}, &model.Client{}, &model.User{}}
	for _, table := range tables {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Client{}, &model.CurrentAccount{}, &model.CreditLine{},
		&model.LivestockQuotaAssessment{}, &model.LivestockCreditLine{}, &model.LivestockCreditLineDocument{}))
	repo := NewLivestockCreditLineRepository(db)

	newLine := func(note string) (*model.LivestockCreditLine, *model.LivestockCreditLineDocument) {
		assessment := &model.LivestockQuotaAssessment{ID: uuid.New(), NationalID: "17788090", Quota: decimal.NewFromInt(1000000), Currency: "COP"}
		require.NoError(t, db.Create(assessment).Error)
		return model.NewLivestockCreditLine(model.LivestockCreditActivation{AssessmentID: assessment.ID, NationalID: "17788090",
			FullName: "GANADERO", Amount: decimal.NewFromInt(1000000), Currency: "COP", PromissoryNoteNumber: note,
			PartnerReference: note, SignedAt: time.Now(), ActivatedAt: time.Now(), DocumentSHA256: "00", Document: []byte("%PDF")}, "cuppogan")
	}
	enrollment := func(existing *model.Client) *model.LivestockClientEnrollment {
		e := &model.LivestockClientEnrollment{Client: existing}
		if existing == nil {
			user, err := model.NewUser(uuid.NewString(), "17788090@cuppogan.com", model.RoleClient)
			require.NoError(t, err)
			client, err := model.NewClient(user.ID, "GANADERO", "DE PRUEBA", "17788090", "17788090-7",
				model.GenericDateOfBirth, "+573001234567", "", "MONTERIA", "CORDOBA", "Colombia", false, "CO")
			require.NoError(t, err)
			e.User, e.Client, e.Account = user, client, model.NewCurrentAccount(client.ID)
		}
		cl, err := model.NewCreditLine(e.Client.ID, decimal.NewFromInt(1000000), decimal.Zero, 1, false)
		require.NoError(t, err)
		cl.Status = model.CreditLineApproved
		e.CreditLine = cl
		return e
	}

	// A new activation stores the client, its user, account and credit line with the line.
	line, doc := newLine("CPG-2026-000010")
	e := enrollment(nil)
	require.NoError(t, repo.Create(ctx, line, doc, e))
	stored, err := repo.FindByID(ctx, line.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.ClientID)
	assert.Equal(t, e.Client.ID, *stored.ClientID)
	assert.Equal(t, e.CreditLine.ID, *stored.CreditLineID)
	var client model.Client
	require.NoError(t, db.Preload("User").First(&client, "id = ?", e.Client.ID).Error)
	assert.Equal(t, "17788090@cuppogan.com", client.User.Email)
	assert.Equal(t, "17788090-7", client.CUIT)
	var creditLine model.CreditLine
	require.NoError(t, db.First(&creditLine, "id = ?", e.CreditLine.ID).Error)
	assert.Equal(t, model.CreditLineApproved, creditLine.Status)
	assert.Equal(t, client.ID, creditLine.ClientID)
	var accounts int64
	db.Model(&model.CurrentAccount{}).Where("client_id = ?", client.ID).Count(&accounts)
	assert.EqualValues(t, 1, accounts)

	// A failing enrollment stores nothing: a second user with the same e-mail.
	line2, doc2 := newLine("CPG-2026-000011")
	require.Error(t, repo.Create(ctx, line2, doc2, enrollment(nil)))
	missing, err := repo.FindByID(ctx, line2.ID)
	require.NoError(t, err)
	assert.Nil(t, missing, "the line is not stored when the enrollment fails")

	// A line stored without client is linked later; a known client only gets the credit line.
	line3, doc3 := newLine("CPG-2026-000012")
	require.NoError(t, repo.Create(ctx, line3, doc3, nil))
	require.NoError(t, repo.LinkClient(ctx, line3, enrollment(&client)))
	linked, err := repo.FindByID(ctx, line3.ID)
	require.NoError(t, err)
	assert.Equal(t, client.ID, *linked.ClientID)
	var lines int64
	db.Model(&model.CreditLine{}).Where("client_id = ?", client.ID).Count(&lines)
	assert.EqualValues(t, 2, lines)
	var clients int64
	db.Model(&model.Client{}).Count(&clients)
	assert.EqualValues(t, 1, clients)
}
