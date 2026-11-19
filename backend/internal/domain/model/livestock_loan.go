package model

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// LivestockLoanRequest is the loan a partner opens on an activated quota, on
// the terms it agreed with the rancher: the total, the rate (and how it is
// stated), the monthly installments and the amortization system. Prestia
// computes and keeps the schedule; the first installment is due a month after
// the loan is opened.
type LivestockLoanRequest struct {
	// PartnerReference is the activation's reference (CuppoGan's onboarding).
	PartnerReference string
	Total            decimal.Decimal
	Rate             decimal.Decimal
	RateType         RateType
	Installments     int
	System           AmortizationType
}

// MaxLivestockLoanInstallments matches CreditLine's own limit.
const MaxLivestockLoanInstallments = 60

// Validate checks the request on its own.
func (r LivestockLoanRequest) Validate() error {
	invalid := func(code, format string, args ...any) error {
		return &ActivationRejection{Kind: ActivationInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
	}
	switch {
	case strings.TrimSpace(r.PartnerReference) == "":
		return invalid("missing_partner_reference", "partnerReference is required")
	case !r.Total.IsPositive():
		return invalid("invalid_total", "total must be positive")
	case r.Rate.IsNegative():
		return invalid("invalid_rate", "rate cannot be negative")
	case r.Installments < 1 || r.Installments > MaxLivestockLoanInstallments:
		return invalid("invalid_installments", "installments must be between 1 and %d", MaxLivestockLoanInstallments)
	case r.System != AmortizationFrench && r.System != AmortizationGerman:
		return invalid("invalid_system", "system must be french or german")
	}
	if _, err := ParseRateType(string(r.RateType)); err != nil {
		return invalid("invalid_rate_type", "%s", err.Error())
	}
	return nil
}

// Matches tells whether a loan already opened on the line is this request: a
// replay gets that loan back, anything else is a conflict.
func (r LivestockLoanRequest) Matches(line *LivestockCreditLine, loan *Loan) bool {
	return loan.Principal.Equal(r.Total) &&
		loan.NumInstallments == r.Installments &&
		loan.AmortizationType == r.System &&
		line.LoanRateType == string(r.RateType) &&
		line.LoanAgreedRate.Valid && line.LoanAgreedRate.Decimal.Equal(r.Rate)
}
