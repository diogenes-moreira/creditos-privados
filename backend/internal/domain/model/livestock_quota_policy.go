package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// This file makes the livestock quota engine's assumptions operator-editable.
//
// Every number the engine takes used to be a constant in DefaultQuotaParams,
// which meant a price move or a policy decision needed a deployment. A
// LivestockQuotaPolicy is the same set of assumptions stored as data: named,
// versioned, validated, and with exactly one active at a time, the same way a
// RiskModel carries credit-risk policy.
//
// The engine's math is untouched. A policy only supplies QuotaParams; when no
// policy is active the engine keeps using DefaultQuotaParams().

// LivestockQuotaPolicy is one versioned set of livestock quota parameters.
type LivestockQuotaPolicy struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name    string    `gorm:"not null"`
	Version int       `gorm:"not null;default:1"`

	Description string `gorm:"type:text"`
	IsActive    bool   `gorm:"not null;default:false;index"`

	// Sale prices per kilo and reference live weights per category.
	PriceCalfPerKg      decimal.Decimal `gorm:"type:decimal(18,4);not null"`
	PriceGrowingPerKg   decimal.Decimal `gorm:"type:decimal(18,4);not null"`
	PriceFatteningPerKg decimal.Decimal `gorm:"type:decimal(18,4);not null"`
	WeightCalfKg        decimal.Decimal `gorm:"type:decimal(12,4);not null"`
	WeightGrowingKg     decimal.Decimal `gorm:"type:decimal(12,4);not null"`
	WeightFatteningKg   decimal.Decimal `gorm:"type:decimal(12,4);not null"`

	// Milk: the share of breeding females in lactation and the price per litre.
	MilkingShare  decimal.Decimal `gorm:"type:decimal(6,4);not null"`
	PricePerLitre decimal.Decimal `gorm:"type:decimal(18,4);not null"`

	// Annual operating costs per UGG, plus the day rate hired labour is paid.
	VaccineCostPerUGGYear    decimal.Decimal `gorm:"column:vaccine_cost_per_ugg_year;type:decimal(18,4);not null"`
	SaltCostPerUGGYear       decimal.Decimal `gorm:"column:salt_cost_per_ugg_year;type:decimal(18,4);not null"`
	SupplementCostPerUGGYear decimal.Decimal `gorm:"column:supplement_cost_per_ugg_year;type:decimal(18,4);not null"`
	VetCostPerUGGYear        decimal.Decimal `gorm:"column:vet_cost_per_ugg_year;type:decimal(18,4);not null"`
	DailyWage                decimal.Decimal `gorm:"type:decimal(18,4);not null"`

	// Labour regime and the UGG thresholds that switch it.
	LabourRegime           LabourRegime    `gorm:"type:varchar(20);not null;default:'auto'"`
	UGGThresholdFamily     decimal.Decimal `gorm:"column:ugg_threshold_family;type:decimal(12,4);not null"`
	UGGThresholdOccasional decimal.Decimal `gorm:"column:ugg_threshold_occasional;type:decimal(12,4);not null"`
	UGGThresholdPartial    decimal.Decimal `gorm:"column:ugg_threshold_partial;type:decimal(12,4);not null"`
	OccasionalDaysPerMonth int             `gorm:"not null;default:4"`

	// Upkeep: fencing per hectare, infrastructure per UGG, stocking rate.
	FenceCostPerHaYear  decimal.Decimal `gorm:"type:decimal(18,4);not null"`
	InfraCostPerUGGYear decimal.Decimal `gorm:"column:infra_cost_per_ugg_year;type:decimal(18,4);not null"`
	HectaresPerUGG      decimal.Decimal `gorm:"column:hectares_per_ugg;type:decimal(12,4);not null"`

	// Credit policy.
	LTVPercent        decimal.Decimal `gorm:"column:ltv_percent;type:decimal(8,4);not null"`
	AnnualRatePct     decimal.Decimal `gorm:"type:decimal(8,4);not null"`
	CapitalCostPct    decimal.Decimal `gorm:"type:decimal(8,4);not null"`
	MaxQuota          decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	TermMonths        int             `gorm:"not null;default:3"`
	ApplyInputCeiling bool            `gorm:"not null;default:true"`
	InputHeadroom     decimal.Decimal `gorm:"type:decimal(8,4);not null;default:1"`

	// Factors is the typology x category UGG conversion matrix.
	Factors []LivestockUGGFactor `gorm:"foreignKey:PolicyID"`

	CreatedBy *uuid.UUID     `gorm:"type:uuid"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// LivestockUGGFactor is one typology's row of the UGG conversion matrix: how
// many Unidad Gran Ganado each head of a commercial category counts for.
type LivestockUGGFactor struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey"`
	PolicyID uuid.UUID         `gorm:"type:uuid;index;not null"`
	Typology LivestockTypology `gorm:"type:varchar(40);not null"`

	Calves    decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`
	Growing   decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`
	Breeding  decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`
	Fattening decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`
	Cull      decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// OrderedTypologies lists the typologies in the order the matrix is presented.
var OrderedTypologies = []LivestockTypology{
	TypologySpecializedDairy,
	TypologyDualPurpose,
	TypologyCowCalf,
	TypologyFattening,
}

func validTypology(t LivestockTypology) bool {
	for _, known := range OrderedTypologies {
		if known == t {
			return true
		}
	}
	return false
}

// NewLivestockQuotaPolicy creates a validated, inactive policy at version 1.
func NewLivestockQuotaPolicy(name, description string, params QuotaParams, createdBy *uuid.UUID) (*LivestockQuotaPolicy, error) {
	p := &LivestockQuotaPolicy{
		ID:          uuid.New(),
		Name:        strings.TrimSpace(name),
		Version:     1,
		Description: description,
		CreatedBy:   createdBy,
	}
	p.applyParams(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// DefaultLivestockQuotaPolicy is the shipped CuppoGan pilot policy, ready to be
// stored so an operator has something to edit from the first day.
func DefaultLivestockQuotaPolicy(createdBy *uuid.UUID) (*LivestockQuotaPolicy, error) {
	return NewLivestockQuotaPolicy(
		"CuppoGan",
		"Shipped pilot parameters (committee, September 2026).",
		DefaultQuotaParams(),
		createdBy,
	)
}

// NextVersion derives a new inactive version from this policy. The caller
// persists the result; the receiver is left untouched so the running version
// keeps serving until the new one is activated.
func (p *LivestockQuotaPolicy) NextVersion(description string, params QuotaParams, createdBy *uuid.UUID) (*LivestockQuotaPolicy, error) {
	next := &LivestockQuotaPolicy{
		ID:          uuid.New(),
		Name:        p.Name,
		Version:     p.Version + 1,
		Description: description,
		CreatedBy:   createdBy,
	}
	next.applyParams(params)
	if err := next.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

// UpdateParams replaces the policy's parameters in place, rolling back when the
// replacement does not validate: a bad parameter must never reach the engine.
func (p *LivestockQuotaPolicy) UpdateParams(params QuotaParams) error {
	snapshot := p.Params()
	p.applyParams(params)
	if err := p.Validate(); err != nil {
		p.applyParams(snapshot)
		return err
	}
	return nil
}

// Activate marks this policy as the one the engine uses. The caller is
// responsible for deactivating the previously active policy.
func (p *LivestockQuotaPolicy) Activate() { p.IsActive = true }

// Deactivate takes the policy out of service.
func (p *LivestockQuotaPolicy) Deactivate() { p.IsActive = false }

func (p *LivestockQuotaPolicy) applyParams(q QuotaParams) {
	p.PriceCalfPerKg = q.PriceCalfPerKg
	p.PriceGrowingPerKg = q.PriceGrowingPerKg
	p.PriceFatteningPerKg = q.PriceFatteningPerKg
	p.WeightCalfKg = q.WeightCalfKg
	p.WeightGrowingKg = q.WeightGrowingKg
	p.WeightFatteningKg = q.WeightFatteningKg

	p.MilkingShare = q.MilkingShare
	p.PricePerLitre = q.PricePerLitre

	p.VaccineCostPerUGGYear = q.VaccineCostPerUGGYear
	p.SaltCostPerUGGYear = q.SaltCostPerUGGYear
	p.SupplementCostPerUGGYear = q.SupplementCostPerUGGYear
	p.VetCostPerUGGYear = q.VetCostPerUGGYear
	p.DailyWage = q.DailyWage

	p.LabourRegime = q.LabourRegime
	p.UGGThresholdFamily = q.UGGThresholdFamily
	p.UGGThresholdOccasional = q.UGGThresholdOccasional
	p.UGGThresholdPartial = q.UGGThresholdPartial
	p.OccasionalDaysPerMonth = q.OccasionalDaysPerMonth

	p.FenceCostPerHaYear = q.FenceCostPerHaYear
	p.InfraCostPerUGGYear = q.InfraCostPerUGGYear
	p.HectaresPerUGG = q.HectaresPerUGG

	p.LTVPercent = q.LTVPercent
	p.AnnualRatePct = q.AnnualRatePct
	p.CapitalCostPct = q.CapitalCostPct
	p.MaxQuota = q.MaxQuota
	p.TermMonths = q.TermMonths
	p.ApplyInputCeiling = q.ApplyInputCeiling
	p.InputHeadroom = q.InputHeadroom

	p.setFactors(q.UGGFactors)
}

func (p *LivestockQuotaPolicy) setFactors(matrix map[LivestockTypology]UGGFactors) {
	if len(matrix) == 0 {
		matrix = DefaultUGGFactors()
	}
	rows := make([]LivestockUGGFactor, 0, len(matrix))
	for typology, f := range matrix {
		rows = append(rows, LivestockUGGFactor{
			ID:        uuid.New(),
			PolicyID:  p.ID,
			Typology:  typology,
			Calves:    f.Calves,
			Growing:   f.Growing,
			Breeding:  f.Breeding,
			Fattening: f.Fattening,
			Cull:      f.Cull,
		})
	}
	// Stable presentation order, so the stored matrix and the editor agree.
	rank := map[LivestockTypology]int{}
	for i, t := range OrderedTypologies {
		rank[t] = i
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, okI := rank[rows[i].Typology]
		rj, okJ := rank[rows[j].Typology]
		if okI != okJ {
			return okI
		}
		if ri != rj {
			return ri < rj
		}
		return rows[i].Typology < rows[j].Typology
	})
	p.Factors = rows
}

// FactorMatrix returns the policy's UGG factors as the engine's matrix.
func (p *LivestockQuotaPolicy) FactorMatrix() map[LivestockTypology]UGGFactors {
	if len(p.Factors) == 0 {
		return DefaultUGGFactors()
	}
	matrix := make(map[LivestockTypology]UGGFactors, len(p.Factors))
	for _, f := range p.Factors {
		matrix[f.Typology] = UGGFactors{
			Calves:    f.Calves,
			Growing:   f.Growing,
			Breeding:  f.Breeding,
			Fattening: f.Fattening,
			Cull:      f.Cull,
		}
	}
	return matrix
}

// Params returns the stored parameters in the shape the engine consumes.
func (p *LivestockQuotaPolicy) Params() QuotaParams {
	return QuotaParams{
		PriceCalfPerKg:      p.PriceCalfPerKg,
		PriceGrowingPerKg:   p.PriceGrowingPerKg,
		PriceFatteningPerKg: p.PriceFatteningPerKg,
		WeightCalfKg:        p.WeightCalfKg,
		WeightGrowingKg:     p.WeightGrowingKg,
		WeightFatteningKg:   p.WeightFatteningKg,

		MilkingShare:  p.MilkingShare,
		PricePerLitre: p.PricePerLitre,

		VaccineCostPerUGGYear:    p.VaccineCostPerUGGYear,
		SaltCostPerUGGYear:       p.SaltCostPerUGGYear,
		SupplementCostPerUGGYear: p.SupplementCostPerUGGYear,
		VetCostPerUGGYear:        p.VetCostPerUGGYear,
		DailyWage:                p.DailyWage,

		LabourRegime:           p.LabourRegime,
		UGGThresholdFamily:     p.UGGThresholdFamily,
		UGGThresholdOccasional: p.UGGThresholdOccasional,
		UGGThresholdPartial:    p.UGGThresholdPartial,
		OccasionalDaysPerMonth: p.OccasionalDaysPerMonth,

		FenceCostPerHaYear:  p.FenceCostPerHaYear,
		InfraCostPerUGGYear: p.InfraCostPerUGGYear,
		HectaresPerUGG:      p.HectaresPerUGG,

		LTVPercent:        p.LTVPercent,
		AnnualRatePct:     p.AnnualRatePct,
		CapitalCostPct:    p.CapitalCostPct,
		MaxQuota:          p.MaxQuota,
		TermMonths:        p.TermMonths,
		ApplyInputCeiling: p.ApplyInputCeiling,
		InputHeadroom:     p.InputHeadroom,

		UGGFactors: p.FactorMatrix(),
	}
}

// maxTermMonths bounds the credit term. The pilot runs at three months with
// principal at maturity; five years is a generous ceiling that still rejects an
// operator typo such as 360.
const maxTermMonths = 60

// Validate enforces the invariants that protect the quota math. A parameter
// that makes no sense is rejected rather than stored: a bad number here does
// not fail loudly, it silently produces a wrong credit limit.
func (p *LivestockQuotaPolicy) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("policy name is required")
	}

	nonNegative := []struct {
		name  string
		value decimal.Decimal
	}{
		{"calf price per kg", p.PriceCalfPerKg},
		{"growing price per kg", p.PriceGrowingPerKg},
		{"fattening price per kg", p.PriceFatteningPerKg},
		{"calf weight", p.WeightCalfKg},
		{"growing weight", p.WeightGrowingKg},
		{"fattening weight", p.WeightFatteningKg},
		{"price per litre", p.PricePerLitre},
		{"vaccine cost per UGG", p.VaccineCostPerUGGYear},
		{"salt cost per UGG", p.SaltCostPerUGGYear},
		{"supplement cost per UGG", p.SupplementCostPerUGGYear},
		{"vet cost per UGG", p.VetCostPerUGGYear},
		{"daily wage", p.DailyWage},
		{"fence cost per hectare", p.FenceCostPerHaYear},
		{"infrastructure cost per UGG", p.InfraCostPerUGGYear},
		{"hectares per UGG", p.HectaresPerUGG},
		{"LTV percent", p.LTVPercent},
		{"annual rate", p.AnnualRatePct},
		{"capital cost", p.CapitalCostPct},
		{"input headroom", p.InputHeadroom},
		{"family labour threshold", p.UGGThresholdFamily},
	}
	for _, f := range nonNegative {
		if f.value.IsNegative() {
			return fmt.Errorf("%s cannot be negative", f.name)
		}
	}

	if p.MilkingShare.IsNegative() || p.MilkingShare.GreaterThan(decimal.NewFromInt(1)) {
		return fmt.Errorf("milking share must be a fraction between 0 and 1")
	}
	if !p.MaxQuota.IsPositive() {
		return fmt.Errorf("maximum quota must be positive")
	}
	if p.TermMonths < 1 || p.TermMonths > maxTermMonths {
		return fmt.Errorf("term months must be between 1 and %d", maxTermMonths)
	}
	if p.OccasionalDaysPerMonth < 0 || p.OccasionalDaysPerMonth > 31 {
		return fmt.Errorf("occasional days per month must be between 0 and 31")
	}
	switch p.LabourRegime {
	case LabourAuto, LabourFamily, LabourFormal, "":
	default:
		return fmt.Errorf("unknown labour regime: %s", p.LabourRegime)
	}

	// The automatic regime walks the thresholds in order, so they must be
	// strictly increasing: equal or inverted bounds silently skip a regime and
	// charge the wrong wage bill.
	if !p.UGGThresholdOccasional.GreaterThan(p.UGGThresholdFamily) {
		return fmt.Errorf("the occasional labour threshold must be greater than the family one")
	}
	if !p.UGGThresholdPartial.GreaterThan(p.UGGThresholdOccasional) {
		return fmt.Errorf("the part-time labour threshold must be greater than the occasional one")
	}

	return p.validateFactors()
}

func (p *LivestockQuotaPolicy) validateFactors() error {
	if len(p.Factors) == 0 {
		return fmt.Errorf("the UGG factor matrix is required")
	}
	seen := map[LivestockTypology]bool{}
	for _, f := range p.Factors {
		if !validTypology(f.Typology) {
			return fmt.Errorf("unknown typology in the UGG matrix: %s", f.Typology)
		}
		if seen[f.Typology] {
			return fmt.Errorf("duplicate typology in the UGG matrix: %s", f.Typology)
		}
		seen[f.Typology] = true

		for _, cell := range []struct {
			name  string
			value decimal.Decimal
		}{
			{"calves", f.Calves},
			{"growing", f.Growing},
			{"breeding", f.Breeding},
			{"fattening", f.Fattening},
			{"cull", f.Cull},
		} {
			if cell.value.IsNegative() {
				return fmt.Errorf("UGG factor %s/%s cannot be negative", f.Typology, cell.name)
			}
		}
	}
	for _, typology := range OrderedTypologies {
		if !seen[typology] {
			return fmt.Errorf("the UGG matrix is missing the %s typology", typology)
		}
	}
	return nil
}
