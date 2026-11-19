package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type loanCreditLines struct {
	port.CreditLineRepository
	byID map[uuid.UUID]*model.CreditLine
}

func (f *loanCreditLines) FindByID(_ context.Context, id uuid.UUID) (*model.CreditLine, error) {
	if cl, ok := f.byID[id]; ok {
		copy := *cl
		return &copy, nil
	}
	return nil, errors.New("credit line not found")
}

type loanRepo struct {
	port.LoanRepository
	lines *fakeCreditLines
}

func (f *loanRepo) FindByIDWithInstallments(_ context.Context, id uuid.UUID) (*model.Loan, error) {
	for _, l := range f.lines.loans {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, errors.New("loan not found")
}

// activatedForLoans is an activation already enrolled (client and credit line).
func activatedForLoans(t *testing.T) (*service.LivestockCreditLineService, *fakeCreditLines, model.LivestockLoanRequest) {
	t.Helper()
	s, lines, _, _, a := enrollingFixture()
	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	cl := lines.enrollments[0].CreditLine
	s.WithLoans(&loanCreditLines{byID: map[uuid.UUID]*model.CreditLine{cl.ID: cl}}, &loanRepo{lines: lines})
	return s, lines, model.LivestockLoanRequest{PartnerReference: a.PartnerReference,
		Total: decimal.RequireFromString("1000000"), Rate: decimal.RequireFromString("26.01"),
		RateType: model.RateEffectiveAnnual, Installments: 12, System: model.AmortizationFrench}
}

func TestOpenLoanComputesTheScheduleOnTheEffectiveRate(t *testing.T) {
	s, lines, req := activatedForLoans(t)

	loan, line, created, err := s.OpenLoan(context.Background(), "cuppogan", req)

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, model.LoanActive, loan.Status)
	assert.Nil(t, loan.ApprovedBy, "approved on the partner's terms, by no back-office user")
	assert.Equal(t, "23.3433", loan.InterestRate.StringFixed(4))
	assert.Equal(t, *line.ClientID, loan.ClientID)
	assert.Equal(t, *line.CreditLineID, loan.CreditLineID)
	require.Len(t, loan.Installments, 12)
	first := loan.Installments[0]
	assert.True(t, first.IVAAmount.IsZero(), "no VAT on the interest")
	assert.Equal(t, loan.DisbursedAt.AddDate(0, 1, 0).Format("2006-01-02"), first.DueDate.Format("2006-01-02"))
	assert.Equal(t, "94242.08", first.TotalAmount.StringFixed(2), "French payment at 1.945275 % a month")
	assert.Equal(t, "19452.75", first.InterestAmount.StringFixed(2))

	assert.Equal(t, loan.ID, *line.LoanID)
	assert.Equal(t, "26.01", line.LoanAgreedRate.Decimal.String())
	assert.Equal(t, "TEA", line.LoanRateType)
	require.Len(t, lines.creditLineUpdates, 1)
	cl := lines.creditLineUpdates[0]
	assert.Equal(t, "23.3433", cl.InterestRate.StringFixed(4), "the line adopts the loan's terms")
	assert.Equal(t, 12, cl.MaxInstallments)
	assert.Equal(t, "1000000.00", cl.UsedAmount.StringFixed(2))
}

func TestOpenLoanIsOnePerActivation(t *testing.T) {
	s, lines, req := activatedForLoans(t)
	first, _, _, err := s.OpenLoan(context.Background(), "cuppogan", req)
	require.NoError(t, err)

	again, _, created, err := s.OpenLoan(context.Background(), "cuppogan", req)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Len(t, lines.loans, 1)

	req.Installments = 6
	_, _, _, err = s.OpenLoan(context.Background(), "cuppogan", req)
	assert.Equal(t, "loan_exists", code(t, err))
}

func TestOpenLoanRejections(t *testing.T) {
	s, _, req := activatedForLoans(t)

	over := req
	over.Total = decimal.RequireFromString("1000000.01")
	_, _, _, err := s.OpenLoan(context.Background(), "cuppogan", over)
	assert.Equal(t, "exceeds_credit_line", code(t, err))

	unknown := req
	unknown.PartnerReference = "another-onboarding"
	_, _, _, err = s.OpenLoan(context.Background(), "cuppogan", unknown)
	assert.Equal(t, "unknown_activation", code(t, err))

	for name, c := range map[string]func(*model.LivestockLoanRequest){
		"invalid_installments": func(r *model.LivestockLoanRequest) { r.Installments = 0 },
		"invalid_system":       func(r *model.LivestockLoanRequest) { r.System = "american" },
		"invalid_rate_type":    func(r *model.LivestockLoanRequest) { r.RateType = "NAMV" },
		"invalid_total":        func(r *model.LivestockLoanRequest) { r.Total = decimal.Zero },
	} {
		bad := req
		c(&bad)
		_, _, _, err := s.OpenLoan(context.Background(), "cuppogan", bad)
		assert.Equal(t, name, code(t, err))
	}
}

func TestOpenLoanNeedsAnEnrolledActivation(t *testing.T) {
	s, lines, a := creditFixture()
	a.MobilePhone = "+573001234567"
	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	s.WithLoans(&loanCreditLines{byID: map[uuid.UUID]*model.CreditLine{}}, &loanRepo{lines: lines})

	_, _, _, err = s.OpenLoan(context.Background(), "cuppogan", model.LivestockLoanRequest{PartnerReference: a.PartnerReference,
		Total: decimal.NewFromInt(1000), Rate: decimal.NewFromInt(10), RateType: model.RateEffectiveAnnual, Installments: 1,
		System: model.AmortizationFrench})
	assert.Equal(t, "not_enrolled", code(t, err))
}
