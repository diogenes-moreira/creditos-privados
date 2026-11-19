package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNominalAnnualRateFromTEA(t *testing.T) {
	nominal, err := NominalAnnualRate(decimal.RequireFromString("26.01"), RateEffectiveAnnual)
	require.NoError(t, err)
	assert.Equal(t, "23.3433", nominal.StringFixed(4))

	same, err := NominalAnnualRate(decimal.RequireFromString("24"), RateNominalAnnual)
	require.NoError(t, err)
	assert.Equal(t, "24.0000", same.StringFixed(4))

	zero, err := NominalAnnualRate(decimal.Zero, RateEffectiveAnnual)
	require.NoError(t, err)
	assert.True(t, zero.IsZero())

	_, err = NominalAnnualRate(decimal.NewFromInt(-1), RateEffectiveAnnual)
	assert.Error(t, err)
}

func TestTEAScheduleMatchesTheEffectiveRate(t *testing.T) {
	// 12 French installments at 26.01 % TEA: the payment of the monthly
	// effective rate (1.945271 %), not of 26.01 / 12 (124,115). The stored rate keeps
	// 4 decimals: 3 cents above the exact 122,435.04.
	nominal, _ := NominalAnnualRate(decimal.RequireFromString("26.01"), RateEffectiveAnnual)
	s := CalculateFrenchAmortization(decimal.RequireFromString("1299155"), nominal, 12, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), decimal.Zero)
	assert.Equal(t, "122435.07", s.Installments[0].Total.StringFixed(2))
	assert.True(t, s.TotalIVA.IsZero())
}

func TestParseRateType(t *testing.T) {
	r, err := ParseRateType("tea")
	require.NoError(t, err)
	assert.Equal(t, RateEffectiveAnnual, r)
	_, err = ParseRateType("NAMV")
	assert.Error(t, err)
}
