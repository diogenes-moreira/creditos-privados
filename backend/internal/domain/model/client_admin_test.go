package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAdminTestClient builds a valid Argentine client for admin-side tests.
func newAdminTestClient(t *testing.T) *model.Client {
	t.Helper()
	c, err := model.NewClient(
		uuid.New(),
		"Ana", "Gómez",
		validDNI(), validCUIT(),
		validDOB(),
		"1130000000", "Av. Corrientes 1000", "CABA", "Buenos Aires", "Argentina",
		false,
		"AR",
	)
	require.NoError(t, err)
	return c
}

func TestClientSetIVARate(t *testing.T) {
	c := newAdminTestClient(t)

	c.SetIVARate(decimal.NewFromFloat(10.5))
	assert.True(t, c.IVARate.Equal(decimal.NewFromFloat(10.5)))

	// Colombia's rate, and zero for exempt clients.
	c.SetIVARate(decimal.NewFromInt(19))
	assert.True(t, c.IVARate.Equal(decimal.NewFromInt(19)))

	c.SetIVARate(decimal.Zero)
	assert.True(t, c.IVARate.IsZero())
}

func TestClientSetComments(t *testing.T) {
	c := newAdminTestClient(t)

	c.SetComments("Called on 2026-08-31, promised payment")
	assert.Equal(t, "Called on 2026-08-31, promised payment", c.Comments)

	// Comments are replaced wholesale, including clearing them.
	c.SetComments("")
	assert.Equal(t, "", c.Comments)
}

func TestClientAdminUpdate(t *testing.T) {
	t.Run("updates every provided field", func(t *testing.T) {
		c := newAdminTestClient(t)

		c.AdminUpdate(
			"Beatriz", "Fernández",
			"87654321", "27-87654321-4",
			"1145550000", "Callao 500", "Rosario", "Santa Fe", "Colombia",
			true,
		)

		assert.Equal(t, "Beatriz", c.FirstName)
		assert.Equal(t, "Fernández", c.LastName)
		assert.Equal(t, "87654321", c.DNI)
		assert.Equal(t, "27-87654321-4", c.CUIT)
		assert.Equal(t, "1145550000", c.Phone)
		assert.Equal(t, "Callao 500", c.Address)
		assert.Equal(t, "Rosario", c.City)
		assert.Equal(t, "Santa Fe", c.Province)
		assert.Equal(t, "Colombia", c.Country)
		assert.True(t, c.IsPEP)
	})

	t.Run("empty strings leave existing values untouched", func(t *testing.T) {
		c := newAdminTestClient(t)
		before := *c

		c.AdminUpdate("", "", "", "", "", "", "", "", "", false)

		assert.Equal(t, before.FirstName, c.FirstName)
		assert.Equal(t, before.LastName, c.LastName)
		assert.Equal(t, before.DNI, c.DNI)
		assert.Equal(t, before.CUIT, c.CUIT)
		assert.Equal(t, before.Phone, c.Phone)
		assert.Equal(t, before.Address, c.Address)
		assert.Equal(t, before.City, c.City)
		assert.Equal(t, before.Province, c.Province)
		assert.Equal(t, before.Country, c.Country)
	})

	t.Run("isPEP is always assigned, not merged", func(t *testing.T) {
		c := newAdminTestClient(t)

		// Turning the flag on with no other edits.
		c.AdminUpdate("", "", "", "", "", "", "", "", "", true)
		assert.True(t, c.IsPEP)

		// And back off again — false is a real value here, not "unset".
		c.AdminUpdate("", "", "", "", "", "", "", "", "", false)
		assert.False(t, c.IsPEP)
	})

	t.Run("partial update only touches named fields", func(t *testing.T) {
		c := newAdminTestClient(t)
		originalDNI := c.DNI

		c.AdminUpdate("", "", "", "", "1199998888", "", "Córdoba", "", "", false)

		assert.Equal(t, "1199998888", c.Phone)
		assert.Equal(t, "Córdoba", c.City)
		assert.Equal(t, originalDNI, c.DNI, "DNI must survive a partial update")
	})
}

func TestNewAuditLog(t *testing.T) {
	t.Run("records the acting user and request context", func(t *testing.T) {
		userID := uuid.New()

		log := model.NewAuditLog(
			&userID,
			"loan.approve", "Loan", "loan-123",
			"Approved loan for 120000",
			"203.0.113.7", "Mozilla/5.0",
		)

		require.NotNil(t, log)
		assert.NotEqual(t, uuid.Nil, log.ID)
		require.NotNil(t, log.UserID)
		assert.Equal(t, userID, *log.UserID)
		assert.Equal(t, "loan.approve", log.Action)
		assert.Equal(t, "Loan", log.EntityType)
		assert.Equal(t, "loan-123", log.EntityID)
		assert.Equal(t, "Approved loan for 120000", log.Description)
		assert.Equal(t, "203.0.113.7", log.IP)
		assert.Equal(t, "Mozilla/5.0", log.UserAgent)
		assert.False(t, log.CreatedAt.IsZero(), "audit entries must be timestamped")
	})

	t.Run("supports system actions with no user", func(t *testing.T) {
		log := model.NewAuditLog(nil, "cron.accrue", "Loan", "", "Nightly accrual", "", "")

		require.NotNil(t, log)
		assert.Nil(t, log.UserID)
		assert.Equal(t, "cron.accrue", log.Action)
		assert.False(t, log.CreatedAt.IsZero())
	})

	t.Run("each entry gets a distinct id", func(t *testing.T) {
		a := model.NewAuditLog(nil, "a", "E", "1", "", "", "")
		b := model.NewAuditLog(nil, "b", "E", "1", "", "", "")

		assert.NotEqual(t, a.ID, b.ID)
	})
}
