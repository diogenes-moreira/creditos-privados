package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// RiskGrade is the risk band an applicant falls into, A (best) to E (worst).
type RiskGrade string

const (
	RiskGradeA RiskGrade = "A"
	RiskGradeB RiskGrade = "B"
	RiskGradeC RiskGrade = "C"
	RiskGradeD RiskGrade = "D"
	RiskGradeE RiskGrade = "E"
)

// OrderedRiskGrades lists grades from best to worst.
var OrderedRiskGrades = []RiskGrade{RiskGradeA, RiskGradeB, RiskGradeC, RiskGradeD, RiskGradeE}

// RiskDecision is the outcome the analysis recommends for an application.
type RiskDecision string

const (
	RiskDecisionApprove      RiskDecision = "approve"
	RiskDecisionManualReview RiskDecision = "manual_review"
	RiskDecisionDecline      RiskDecision = "decline"
)

func (d RiskDecision) valid() bool {
	switch d {
	case RiskDecisionApprove, RiskDecisionManualReview, RiskDecisionDecline:
		return true
	}
	return false
}

// severity ranks decisions so guardrails can only ever tighten an outcome.
func (d RiskDecision) severity() int {
	switch d {
	case RiskDecisionApprove:
		return 0
	case RiskDecisionManualReview:
		return 1
	default:
		return 2
	}
}

// StricterOf returns the more conservative of two decisions.
func StricterOf(a, b RiskDecision) RiskDecision {
	if b.severity() > a.severity() {
		return b
	}
	return a
}

// placeholderPattern matches the {{key}} substitutions allowed in a prompt.
var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_]+)\s*\}\}`)

// KnownPromptPlaceholders are the substitution keys the risk service fills in
// when rendering an analysis prompt. Any other placeholder is a typo and is
// rejected at validation time rather than silently rendered as literal text.
var KnownPromptPlaceholders = []string{
	"client", "profile", "history", "policy", "baseline", "currency", "country", "today",
}

// RiskModel is the credit-risk analysis model. It pairs an operator-editable
// analysis prompt with the numeric policy guardrails that bound whatever the
// analysis proposes. Models are versioned and exactly one is active at a time.
type RiskModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name    string    `gorm:"not null"`
	Version int       `gorm:"not null;default:1"`

	// Prompt is the analysis instruction set, maintained by the operator.
	Prompt      string `gorm:"type:text;not null"`
	Description string `gorm:"type:text"`
	IsActive    bool   `gorm:"not null;default:false;index"`

	// Eligibility guardrails.
	MinAge            int             `gorm:"not null;default:18"`
	MaxAge            int             `gorm:"not null;default:75"`
	MinMonthlyIncome  decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	MaxAdverseRecords int             `gorm:"not null;default:0"`

	// Affordability guardrails. Ratios are fractions of income (0.30 = 30%).
	MaxDTI                  decimal.Decimal `gorm:"column:max_dti;type:decimal(6,4);not null;default:0.45"`
	MaxPaymentToIncome      decimal.Decimal `gorm:"type:decimal(6,4);not null;default:0.30"`
	UnverifiedIncomeHaircut decimal.Decimal `gorm:"type:decimal(6,4);not null;default:0.20"`
	ExpensePerDependent     decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`

	// Exposure guardrails.
	AbsoluteMaxLimit  decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	MaxInstallments   int             `gorm:"not null;default:12"`
	LimitRoundingStep decimal.Decimal `gorm:"type:decimal(18,2);not null;default:1000"`
	// BaseInterestRate is an annual percentage: 60 means 60% per year.
	BaseInterestRate decimal.Decimal `gorm:"type:decimal(8,4);not null"`

	Bands []RiskGradeBand `gorm:"foreignKey:RiskModelID"`

	CreatedBy *uuid.UUID     `gorm:"type:uuid"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// RiskGradeBand maps a score range onto pricing, exposure and a decision.
type RiskGradeBand struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	RiskModelID uuid.UUID `gorm:"type:uuid;index;not null"`
	Grade       RiskGrade `gorm:"type:varchar(2);not null"`

	// MinScore is the inclusive lower bound of the band on a 0-100 scale.
	MinScore int `gorm:"not null"`
	// InterestSpread is added to the model's base interest rate, in the same
	// unit: annual percentage points (5 means +5% per year).
	InterestSpread decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`
	// ExposureFactor scales the affordability-derived limit (0.80 = 80%).
	ExposureFactor decimal.Decimal `gorm:"type:decimal(6,4);not null;default:1"`
	// MaxInstallments caps the term for this band; zero means "model default".
	MaxInstallments int `gorm:"not null;default:0"`
	// Decision is the recommended outcome for applicants in this band.
	Decision RiskDecision `gorm:"type:varchar(20);not null;default:'manual_review'"`
	// ProbabilityOfDefault is the expected default rate for the band.
	ProbabilityOfDefault decimal.Decimal `gorm:"type:decimal(6,4);not null;default:0"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// RiskModelParams carries the editable guardrails of a risk model.
type RiskModelParams struct {
	MinAge                  int
	MaxAge                  int
	MinMonthlyIncome        decimal.Decimal
	MaxAdverseRecords       int
	MaxDTI                  decimal.Decimal
	MaxPaymentToIncome      decimal.Decimal
	UnverifiedIncomeHaircut decimal.Decimal
	ExpensePerDependent     decimal.Decimal
	AbsoluteMaxLimit        decimal.Decimal
	MaxInstallments         int
	LimitRoundingStep       decimal.Decimal
	BaseInterestRate        decimal.Decimal
}

// NewRiskModel creates a validated, inactive risk model at version 1.
func NewRiskModel(name, prompt, description string, params RiskModelParams, bands []RiskGradeBand, createdBy *uuid.UUID) (*RiskModel, error) {
	m := &RiskModel{
		ID:          uuid.New(),
		Name:        name,
		Version:     1,
		Prompt:      prompt,
		Description: description,
		CreatedBy:   createdBy,
	}
	m.applyParams(params)
	m.setBands(bands)
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// NextVersion derives a new inactive model version from this one. The caller
// persists the result; the receiver is left untouched so the running version
// keeps serving until the new one is activated.
func (m *RiskModel) NextVersion(prompt, description string, params RiskModelParams, bands []RiskGradeBand, createdBy *uuid.UUID) (*RiskModel, error) {
	next := &RiskModel{
		ID:          uuid.New(),
		Name:        m.Name,
		Version:     m.Version + 1,
		Prompt:      prompt,
		Description: description,
		CreatedBy:   createdBy,
	}
	next.applyParams(params)
	if len(bands) == 0 {
		bands = m.cloneBands()
	}
	next.setBands(bands)
	if err := next.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func (m *RiskModel) applyParams(p RiskModelParams) {
	m.MinAge = p.MinAge
	m.MaxAge = p.MaxAge
	m.MinMonthlyIncome = p.MinMonthlyIncome
	m.MaxAdverseRecords = p.MaxAdverseRecords
	m.MaxDTI = p.MaxDTI
	m.MaxPaymentToIncome = p.MaxPaymentToIncome
	m.UnverifiedIncomeHaircut = p.UnverifiedIncomeHaircut
	m.ExpensePerDependent = p.ExpensePerDependent
	m.AbsoluteMaxLimit = p.AbsoluteMaxLimit
	m.MaxInstallments = p.MaxInstallments
	m.LimitRoundingStep = p.LimitRoundingStep
	m.BaseInterestRate = p.BaseInterestRate
}

func (m *RiskModel) setBands(bands []RiskGradeBand) {
	out := make([]RiskGradeBand, 0, len(bands))
	for _, b := range bands {
		b.ID = uuid.New()
		b.RiskModelID = m.ID
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MinScore > out[j].MinScore })
	m.Bands = out
}

func (m *RiskModel) cloneBands() []RiskGradeBand {
	out := make([]RiskGradeBand, 0, len(m.Bands))
	for _, b := range m.Bands {
		b.ID = uuid.Nil
		b.RiskModelID = uuid.Nil
		out = append(out, b)
	}
	return out
}

// Params returns the current guardrail values.
func (m *RiskModel) Params() RiskModelParams {
	return RiskModelParams{
		MinAge:                  m.MinAge,
		MaxAge:                  m.MaxAge,
		MinMonthlyIncome:        m.MinMonthlyIncome,
		MaxAdverseRecords:       m.MaxAdverseRecords,
		MaxDTI:                  m.MaxDTI,
		MaxPaymentToIncome:      m.MaxPaymentToIncome,
		UnverifiedIncomeHaircut: m.UnverifiedIncomeHaircut,
		ExpensePerDependent:     m.ExpensePerDependent,
		AbsoluteMaxLimit:        m.AbsoluteMaxLimit,
		MaxInstallments:         m.MaxInstallments,
		LimitRoundingStep:       m.LimitRoundingStep,
		BaseInterestRate:        m.BaseInterestRate,
	}
}

// Validate enforces the invariants of a risk model and its bands.
func (m *RiskModel) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(m.Prompt) == "" {
		return fmt.Errorf("analysis prompt is required")
	}
	if unknown := UnknownPlaceholders(m.Prompt); len(unknown) > 0 {
		return fmt.Errorf("unknown prompt placeholders: %s (allowed: %s)",
			strings.Join(unknown, ", "), strings.Join(KnownPromptPlaceholders, ", "))
	}
	if !UsesPlaceholder(m.Prompt, "profile") {
		return fmt.Errorf("analysis prompt must include the {{profile}} placeholder")
	}
	if m.MinAge < 18 {
		return fmt.Errorf("minimum age cannot be below 18")
	}
	if m.MaxAge <= m.MinAge {
		return fmt.Errorf("maximum age must be greater than minimum age")
	}
	if m.MinMonthlyIncome.IsNegative() {
		return fmt.Errorf("minimum monthly income cannot be negative")
	}
	if m.MaxAdverseRecords < 0 {
		return fmt.Errorf("maximum adverse records cannot be negative")
	}
	if err := validateFraction("max DTI", m.MaxDTI); err != nil {
		return err
	}
	if err := validateFraction("max payment to income", m.MaxPaymentToIncome); err != nil {
		return err
	}
	if err := validateFraction("unverified income haircut", m.UnverifiedIncomeHaircut); err != nil {
		return err
	}
	if !m.MaxPaymentToIncome.IsPositive() {
		return fmt.Errorf("max payment to income must be positive")
	}
	if m.ExpensePerDependent.IsNegative() {
		return fmt.Errorf("expense per dependent cannot be negative")
	}
	if !m.AbsoluteMaxLimit.IsPositive() {
		return fmt.Errorf("absolute max limit must be positive")
	}
	if m.MaxInstallments < 1 || m.MaxInstallments > 60 {
		return fmt.Errorf("max installments must be between 1 and 60")
	}
	if m.LimitRoundingStep.IsNegative() {
		return fmt.Errorf("limit rounding step cannot be negative")
	}
	if m.BaseInterestRate.IsNegative() {
		return fmt.Errorf("base interest rate cannot be negative")
	}
	return m.validateBands()
}

func (m *RiskModel) validateBands() error {
	if len(m.Bands) == 0 {
		return fmt.Errorf("at least one grade band is required")
	}
	seen := map[RiskGrade]bool{}
	lowest := 101
	for _, b := range m.Bands {
		if seen[b.Grade] {
			return fmt.Errorf("duplicate grade band: %s", b.Grade)
		}
		seen[b.Grade] = true
		if b.MinScore < 0 || b.MinScore > 100 {
			return fmt.Errorf("band %s min score must be between 0 and 100", b.Grade)
		}
		if !b.Decision.valid() {
			return fmt.Errorf("band %s has an invalid decision: %s", b.Grade, b.Decision)
		}
		if err := validateFraction(fmt.Sprintf("band %s exposure factor", b.Grade), b.ExposureFactor); err != nil {
			return err
		}
		if err := validateFraction(fmt.Sprintf("band %s probability of default", b.Grade), b.ProbabilityOfDefault); err != nil {
			return err
		}
		if b.InterestSpread.IsNegative() {
			return fmt.Errorf("band %s interest spread cannot be negative", b.Grade)
		}
		if b.MaxInstallments < 0 || b.MaxInstallments > 60 {
			return fmt.Errorf("band %s max installments must be between 0 and 60", b.Grade)
		}
		if b.MinScore < lowest {
			lowest = b.MinScore
		}
	}
	if lowest != 0 {
		return fmt.Errorf("the lowest grade band must start at score 0 so every score maps to a band")
	}
	return nil
}

func validateFraction(name string, v decimal.Decimal) error {
	if v.IsNegative() || v.GreaterThan(decimal.NewFromInt(1)) {
		return fmt.Errorf("%s must be a fraction between 0 and 1", name)
	}
	return nil
}

// PlaceholdersIn returns the distinct placeholder keys a prompt uses, in order
// of first appearance. Surrounding whitespace is tolerated, matching what the
// renderer accepts.
func PlaceholdersIn(prompt string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(prompt, -1) {
		if key := match[1]; !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

// UsesPlaceholder reports whether a prompt references a placeholder key. It
// agrees with the renderer, so {{profile}} and {{ profile }} both count.
func UsesPlaceholder(prompt, key string) bool {
	for _, used := range PlaceholdersIn(prompt) {
		if used == key {
			return true
		}
	}
	return false
}

// UnknownPlaceholders returns the placeholder keys used in a prompt that the
// renderer does not know how to fill.
func UnknownPlaceholders(prompt string) []string {
	allowed := map[string]bool{}
	for _, k := range KnownPromptPlaceholders {
		allowed[k] = true
	}
	var unknown []string
	for _, key := range PlaceholdersIn(prompt) {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	return unknown
}

// RenderPrompt substitutes the {{key}} placeholders with the supplied values.
// Placeholders without a value render as an empty string.
func (m *RiskModel) RenderPrompt(vars map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(m.Prompt, func(match string) string {
		key := placeholderPattern.FindStringSubmatch(match)[1]
		return vars[key]
	})
}

// Activate marks this model as the one used for new evaluations. The caller is
// responsible for deactivating the previously active model.
func (m *RiskModel) Activate() {
	m.IsActive = true
}

// Deactivate takes the model out of service.
func (m *RiskModel) Deactivate() {
	m.IsActive = false
}

// UpdateGuardrails replaces the numeric policy of the model in place.
func (m *RiskModel) UpdateGuardrails(params RiskModelParams) error {
	snapshot := m.Params()
	m.applyParams(params)
	if err := m.Validate(); err != nil {
		m.applyParams(snapshot)
		return err
	}
	return nil
}

// UpdatePrompt replaces the analysis prompt in place.
func (m *RiskModel) UpdatePrompt(prompt string) error {
	previous := m.Prompt
	m.Prompt = prompt
	if err := m.Validate(); err != nil {
		m.Prompt = previous
		return err
	}
	return nil
}

// BandFor returns the band a score falls into, or nil when no band matches.
func (m *RiskModel) BandFor(score int) *RiskGradeBand {
	best := -1
	var found *RiskGradeBand
	for i := range m.Bands {
		b := &m.Bands[i]
		if score >= b.MinScore && b.MinScore > best {
			best = b.MinScore
			found = b
		}
	}
	return found
}

// BandForGrade returns the band matching a grade, or nil when absent.
func (m *RiskModel) BandForGrade(grade RiskGrade) *RiskGradeBand {
	for i := range m.Bands {
		if m.Bands[i].Grade == grade {
			return &m.Bands[i]
		}
	}
	return nil
}

// TermCapFor returns the installment cap that applies to a band.
func (m *RiskModel) TermCapFor(b *RiskGradeBand) int {
	if b == nil || b.MaxInstallments <= 0 || b.MaxInstallments > m.MaxInstallments {
		return m.MaxInstallments
	}
	return b.MaxInstallments
}

// RateFor prices a band as the model base rate plus the band spread.
func (m *RiskModel) RateFor(b *RiskGradeBand) decimal.Decimal {
	if b == nil {
		return m.BaseInterestRate
	}
	return m.BaseInterestRate.Add(b.InterestSpread)
}

// RoundLimit snaps an amount down to the configured rounding step and caps it
// at the absolute exposure limit.
func (m *RiskModel) RoundLimit(amount decimal.Decimal) decimal.Decimal {
	if amount.IsNegative() {
		return decimal.Zero
	}
	if amount.GreaterThan(m.AbsoluteMaxLimit) {
		amount = m.AbsoluteMaxLimit
	}
	if m.LimitRoundingStep.IsPositive() {
		steps := amount.Div(m.LimitRoundingStep).Floor()
		amount = steps.Mul(m.LimitRoundingStep)
	}
	return amount.Round(2)
}
