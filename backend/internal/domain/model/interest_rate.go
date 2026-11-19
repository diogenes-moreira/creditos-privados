package model

import (
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"
)

// RateType is how a partner states an interest rate.
type RateType string

const (
	// RateEffectiveAnnual is the tasa efectiva anual (TEA / E.A.), Colombia's
	// usual way of quoting a credit: 26.01 means the balance grows 26.01 % in a
	// year with monthly compounding.
	RateEffectiveAnnual RateType = "TEA"
	// RateNominalAnnual is Prestia's own convention: an annual percentage
	// divided by 12 for the monthly rate (24 → 2 % a month).
	RateNominalAnnual RateType = "NOMINAL_ANNUAL"
)

// ParseRateType accepts the rate types above (case-insensitive).
func ParseRateType(s string) (RateType, error) {
	switch t := RateType(strings.ToUpper(strings.TrimSpace(s))); t {
	case RateEffectiveAnnual, RateNominalAnnual:
		return t, nil
	default:
		return "", fmt.Errorf("unknown rate type %q (use TEA or NOMINAL_ANNUAL)", s)
	}
}

// NominalAnnualRate converts a rate to the annual percentage Prestia stores in
// CreditLine.InterestRate and Loan.InterestRate (monthly rate × 12), rounded to
// the 4 decimals those columns keep. A TEA r becomes ((1 + r/100)^(1/12) − 1) ×
// 1200: 26.01 % TEA is 1.945271 % a month, 23.3433 nominal.
func NominalAnnualRate(rate decimal.Decimal, t RateType) (decimal.Decimal, error) {
	if rate.IsNegative() {
		return decimal.Zero, fmt.Errorf("interest rate cannot be negative")
	}
	switch t {
	case RateNominalAnnual:
		return rate.Round(4), nil
	case RateEffectiveAnnual:
		r, _ := rate.Div(decimal.NewFromInt(100)).Float64()
		monthly := math.Pow(1+r, 1.0/12) - 1
		return decimal.NewFromFloat(monthly * 1200).Round(4), nil
	default:
		return decimal.Zero, fmt.Errorf("unknown rate type %q", t)
	}
}
