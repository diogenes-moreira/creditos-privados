package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// EmploymentType classifies how the applicant earns their main income.
type EmploymentType string

const (
	EmploymentEmployee      EmploymentType = "employee"
	EmploymentSelfEmployed  EmploymentType = "self_employed"
	EmploymentBusinessOwner EmploymentType = "business_owner"
	EmploymentRetired       EmploymentType = "retired"
	EmploymentInformal      EmploymentType = "informal"
	EmploymentUnemployed    EmploymentType = "unemployed"
)

// HousingStatus describes the applicant's dwelling situation.
type HousingStatus string

const (
	HousingOwned     HousingStatus = "owned"
	HousingMortgaged HousingStatus = "mortgaged"
	HousingRented    HousingStatus = "rented"
	HousingFamily    HousingStatus = "family"
	HousingOther     HousingStatus = "other"
)

// RiskDataSource records which channel supplied the risk data.
type RiskDataSource string

const (
	RiskSourceClient RiskDataSource = "client"
	RiskSourceAdmin  RiskDataSource = "admin"
	RiskSourceVendor RiskDataSource = "vendor"
	RiskSourceAPI    RiskDataSource = "api"
)

// BureauScoreScale is the normalized upper bound for external bureau scores.
// Callers converting from another scale must rescale before storing.
const BureauScoreScale = 1000

// RiskProfile holds the credit-risk data of a client. It is the input side of
// the risk module: an applicant (or an operator, or an API consumer) submits it
// and the analysis model turns it into a credit availability recommendation.
type RiskProfile struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	ClientID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	Client   Client    `gorm:"foreignKey:ClientID"`

	// Income
	MonthlyIncome    decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	AdditionalIncome decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	IncomeVerified   bool            `gorm:"not null;default:false"`

	// Employment
	EmploymentType EmploymentType `gorm:"type:varchar(30);not null"`
	EmployerName   string         `gorm:""`
	MonthsEmployed int            `gorm:"not null;default:0"`

	// Obligations
	MonthlyExpenses      decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	ExistingDebtPayments decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`

	// Household
	Dependents    int           `gorm:"not null;default:0"`
	HousingStatus HousingStatus `gorm:"type:varchar(20);not null;default:'other'"`

	// External bureau data, normalized to BureauScoreScale.
	HasBankAccount bool   `gorm:"not null;default:false"`
	BureauScore    *int   `gorm:""`
	AdverseRecords int    `gorm:"not null;default:0"`
	Notes          string `gorm:"type:text"`

	// Provenance
	Source      RiskDataSource `gorm:"type:varchar(20);not null;default:'client'"`
	SubmittedBy *uuid.UUID     `gorm:"type:uuid"`

	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// RiskProfileInput carries the mutable fields of a risk profile so the
// constructor and the update method share a single validated shape.
type RiskProfileInput struct {
	MonthlyIncome        decimal.Decimal
	AdditionalIncome     decimal.Decimal
	IncomeVerified       bool
	EmploymentType       EmploymentType
	EmployerName         string
	MonthsEmployed       int
	MonthlyExpenses      decimal.Decimal
	ExistingDebtPayments decimal.Decimal
	Dependents           int
	HousingStatus        HousingStatus
	HasBankAccount       bool
	BureauScore          *int
	AdverseRecords       int
	Notes                string
}

// TrustedForUnderwriterFields reports whether a source may set the fields that
// represent an underwriting judgement rather than a self-reported fact.
// Verified income and bureau data are attestations the lender makes; a client
// or a merchant declaring them about themselves would be attesting their own
// creditworthiness.
func (s RiskDataSource) TrustedForUnderwriterFields() bool {
	return s == RiskSourceAdmin || s == RiskSourceAPI
}

// RestrictToSource drops the fields the given source is not trusted to set,
// carrying forward the stored values instead so a self-service update cannot
// clear or fabricate them. existing may be nil for a first submission.
func (in RiskProfileInput) RestrictToSource(source RiskDataSource, existing *RiskProfile) RiskProfileInput {
	if source.TrustedForUnderwriterFields() {
		return in
	}
	if existing == nil {
		in.IncomeVerified = false
		in.BureauScore = nil
		in.AdverseRecords = 0
		return in
	}
	in.IncomeVerified = existing.IncomeVerified
	in.BureauScore = existing.BureauScore
	in.AdverseRecords = existing.AdverseRecords
	return in
}

// NewRiskProfile creates a validated risk profile for a client.
func NewRiskProfile(clientID uuid.UUID, in RiskProfileInput, source RiskDataSource, submittedBy *uuid.UUID) (*RiskProfile, error) {
	p := &RiskProfile{
		ID:          uuid.New(),
		ClientID:    clientID,
		Source:      source,
		SubmittedBy: submittedBy,
	}
	p.apply(in)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Update replaces the profile data, keeping identity and provenance metadata.
func (p *RiskProfile) Update(in RiskProfileInput, source RiskDataSource, submittedBy *uuid.UUID) error {
	snapshot := *p
	p.apply(in)
	if err := p.Validate(); err != nil {
		*p = snapshot
		return err
	}
	p.Source = source
	p.SubmittedBy = submittedBy
	return nil
}

func (p *RiskProfile) apply(in RiskProfileInput) {
	p.MonthlyIncome = in.MonthlyIncome
	p.AdditionalIncome = in.AdditionalIncome
	p.IncomeVerified = in.IncomeVerified
	p.EmploymentType = in.EmploymentType
	p.EmployerName = in.EmployerName
	p.MonthsEmployed = in.MonthsEmployed
	p.MonthlyExpenses = in.MonthlyExpenses
	p.ExistingDebtPayments = in.ExistingDebtPayments
	p.Dependents = in.Dependents
	p.HousingStatus = in.HousingStatus
	p.HasBankAccount = in.HasBankAccount
	p.BureauScore = in.BureauScore
	p.AdverseRecords = in.AdverseRecords
	p.Notes = in.Notes
}

// Validate enforces the invariants of a risk profile.
func (p *RiskProfile) Validate() error {
	if p.MonthlyIncome.IsNegative() {
		return fmt.Errorf("monthly income cannot be negative")
	}
	if p.AdditionalIncome.IsNegative() {
		return fmt.Errorf("additional income cannot be negative")
	}
	if p.MonthlyExpenses.IsNegative() {
		return fmt.Errorf("monthly expenses cannot be negative")
	}
	if p.ExistingDebtPayments.IsNegative() {
		return fmt.Errorf("existing debt payments cannot be negative")
	}
	if p.MonthsEmployed < 0 {
		return fmt.Errorf("months employed cannot be negative")
	}
	if p.Dependents < 0 {
		return fmt.Errorf("dependents cannot be negative")
	}
	if p.AdverseRecords < 0 {
		return fmt.Errorf("adverse records cannot be negative")
	}
	if !p.EmploymentType.valid() {
		return fmt.Errorf("invalid employment type: %s", p.EmploymentType)
	}
	if !p.HousingStatus.valid() {
		return fmt.Errorf("invalid housing status: %s", p.HousingStatus)
	}
	if p.BureauScore != nil && (*p.BureauScore < 0 || *p.BureauScore > BureauScoreScale) {
		return fmt.Errorf("bureau score must be between 0 and %d", BureauScoreScale)
	}
	return nil
}

func (t EmploymentType) valid() bool {
	switch t {
	case EmploymentEmployee, EmploymentSelfEmployed, EmploymentBusinessOwner,
		EmploymentRetired, EmploymentInformal, EmploymentUnemployed:
		return true
	}
	return false
}

func (h HousingStatus) valid() bool {
	switch h {
	case HousingOwned, HousingMortgaged, HousingRented, HousingFamily, HousingOther:
		return true
	}
	return false
}

// GrossMonthlyIncome is the sum of declared main and additional income.
func (p *RiskProfile) GrossMonthlyIncome() decimal.Decimal {
	return p.MonthlyIncome.Add(p.AdditionalIncome)
}

// RecognizedIncome applies a haircut to unverified income. The haircut is a
// fraction (0.20 discounts 20%) and is ignored once income is verified.
func (p *RiskProfile) RecognizedIncome(haircut decimal.Decimal) decimal.Decimal {
	gross := p.GrossMonthlyIncome()
	if p.IncomeVerified || haircut.IsZero() {
		return gross
	}
	factor := decimal.NewFromInt(1).Sub(haircut)
	if factor.IsNegative() {
		return decimal.Zero
	}
	return gross.Mul(factor).Round(2)
}

// TotalMonthlyObligations sums living expenses and external debt service.
func (p *RiskProfile) TotalMonthlyObligations() decimal.Decimal {
	return p.MonthlyExpenses.Add(p.ExistingDebtPayments)
}

// DebtToIncomeRatio is external debt service over gross income. It returns
// zero when no income was declared, since the ratio is undefined there.
func (p *RiskProfile) DebtToIncomeRatio() decimal.Decimal {
	gross := p.GrossMonthlyIncome()
	if !gross.IsPositive() {
		return decimal.Zero
	}
	return p.ExistingDebtPayments.Div(gross).Round(4)
}

// ExpenseRatio is total obligations over gross income.
func (p *RiskProfile) ExpenseRatio() decimal.Decimal {
	gross := p.GrossMonthlyIncome()
	if !gross.IsPositive() {
		return decimal.Zero
	}
	return p.TotalMonthlyObligations().Div(gross).Round(4)
}

// DisposableIncome is recognized income minus obligations and an allowance per
// dependent. It floors at zero: a negative surplus carries no payment capacity.
func (p *RiskProfile) DisposableIncome(haircut, perDependent decimal.Decimal) decimal.Decimal {
	surplus := p.RecognizedIncome(haircut).
		Sub(p.TotalMonthlyObligations()).
		Sub(perDependent.Mul(decimal.NewFromInt(int64(p.Dependents))))
	if surplus.IsNegative() {
		return decimal.Zero
	}
	return surplus.Round(2)
}

// EmploymentYears is months of tenure expressed in years.
func (p *RiskProfile) EmploymentYears() decimal.Decimal {
	return decimal.NewFromInt(int64(p.MonthsEmployed)).Div(decimal.NewFromInt(12)).Round(2)
}

// HasBureauScore reports whether external bureau data is available.
func (p *RiskProfile) HasBureauScore() bool {
	return p.BureauScore != nil
}
