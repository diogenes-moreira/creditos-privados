package model_test

import (
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// newPenaltyInstallment builds an overdue installment with interest and IVA set,
// so penalty surcharges have something to scale.
func newPenaltyInstallment() *model.Installment {
	capital := decimal.NewFromInt(1000)
	interest := decimal.NewFromInt(200)
	iva := decimal.NewFromInt(42) // 21% of interest
	return &model.Installment{
		ID:              uuid.New(),
		LoanID:          uuid.New(),
		Number:          1,
		DueDate:         time.Now().AddDate(0, 0, -30),
		CapitalAmount:   capital,
		InterestAmount:  interest,
		IVAAmount:       iva,
		TotalAmount:     capital.Add(interest).Add(iva),
		PaidAmount:      decimal.Zero,
		RemainingAmount: capital.Add(interest).Add(iva),
		Status:          model.InstallmentPending,
	}
}

func TestInstallmentApplyLatePenalty(t *testing.T) {
	t.Run("surcharges interest and scales iva proportionally", func(t *testing.T) {
		inst := newPenaltyInstallment()

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		// 10% surcharge on 200 interest.
		assert.True(t, inst.InterestAmount.Equal(decimal.NewFromInt(220)),
			"expected 220, got %s", inst.InterestAmount)
		// IVA keeps its proportion: 42 * 220/200 = 46.20
		assert.True(t, inst.IVAAmount.Equal(decimal.NewFromFloat(46.20)),
			"expected 46.20, got %s", inst.IVAAmount)
		// Total and remaining are rebuilt from the new components.
		assert.True(t, inst.TotalAmount.Equal(decimal.NewFromFloat(1266.20)),
			"expected 1266.20, got %s", inst.TotalAmount)
		assert.True(t, inst.RemainingAmount.Equal(inst.TotalAmount))
		assert.True(t, inst.PenaltyApplied)
	})

	t.Run("applies only once", func(t *testing.T) {
		inst := newPenaltyInstallment()

		inst.ApplyLatePenalty(decimal.NewFromInt(10))
		afterFirst := inst.TotalAmount

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		assert.True(t, inst.TotalAmount.Equal(afterFirst),
			"penalty must not compound on repeated calls")
	})

	t.Run("skips paid installments", func(t *testing.T) {
		inst := newPenaltyInstallment()
		inst.Status = model.InstallmentPaid
		before := inst.TotalAmount

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		assert.True(t, inst.TotalAmount.Equal(before))
		assert.False(t, inst.PenaltyApplied)
	})

	t.Run("skips installments that are not yet overdue", func(t *testing.T) {
		inst := newPenaltyInstallment()
		inst.DueDate = time.Now().AddDate(0, 0, 15)
		before := inst.TotalAmount

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		assert.True(t, inst.TotalAmount.Equal(before))
		assert.False(t, inst.PenaltyApplied)
	})

	t.Run("accounts for a partial payment in the remaining amount", func(t *testing.T) {
		inst := newPenaltyInstallment()
		inst.PaidAmount = decimal.NewFromInt(500)
		inst.Status = model.InstallmentPartial
		inst.RemainingAmount = inst.TotalAmount.Sub(inst.PaidAmount)

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		assert.True(t, inst.RemainingAmount.Equal(inst.TotalAmount.Sub(decimal.NewFromInt(500))),
			"remaining should net off what was already paid, got %s", inst.RemainingAmount)
	})

	t.Run("zero interest leaves iva untouched", func(t *testing.T) {
		inst := newPenaltyInstallment()
		inst.InterestAmount = decimal.Zero
		inst.IVAAmount = decimal.Zero
		inst.TotalAmount = inst.CapitalAmount
		inst.RemainingAmount = inst.CapitalAmount

		inst.ApplyLatePenalty(decimal.NewFromInt(10))

		// No interest to surcharge, so the installment is unchanged in value
		// and the IVA scaling is skipped (guarding against divide-by-zero).
		assert.True(t, inst.InterestAmount.IsZero())
		assert.True(t, inst.IVAAmount.IsZero())
		assert.True(t, inst.TotalAmount.Equal(inst.CapitalAmount))
		assert.True(t, inst.PenaltyApplied)
	})

	t.Run("zero penalty rate is a no-op in value", func(t *testing.T) {
		inst := newPenaltyInstallment()
		before := inst.TotalAmount

		inst.ApplyLatePenalty(decimal.Zero)

		assert.True(t, inst.TotalAmount.Equal(before))
		assert.True(t, inst.PenaltyApplied, "the flag still burns so it cannot be re-applied")
	})
}
