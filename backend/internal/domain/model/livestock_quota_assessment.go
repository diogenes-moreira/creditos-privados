package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LivestockQuotaAssessment is one stored run of the livestock quota engine,
// requested by a partner service (CuppoGan) for a rancher identified by
// national ID. The applicant and the full engine result are kept as JSON next
// to the headline figures, so any quota can be explained from this one record.
type LivestockQuotaAssessment struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	NationalID string    `gorm:"type:varchar(30);index;not null"`
	Department string    `gorm:"type:varchar(60)"`
	// RequestedBy is the partner service that asked for the quota.
	RequestedBy string `gorm:"type:varchar(60);not null"`
	// Reference is the caller's own identifier for the request (e.g. CuppoGan's
	// onboarding ID), so both sides can be reconciled.
	Reference string `gorm:"type:varchar(100);index"`

	Quota         decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	Currency      string          `gorm:"type:varchar(3);not null"`
	ActiveCeiling string          `gorm:"type:varchar(40);not null"`
	Score         int             `gorm:"not null"`
	Grade         string          `gorm:"type:varchar(3);not null"`
	Typology      string          `gorm:"type:varchar(40);not null"`
	TermMonths    int             `gorm:"not null"`

	// Strategy is how Quota was decided: QuotaStrategyTiers or
	// QuotaStrategyEngine. Rows stored before strategies existed are empty and
	// were all decided by the engine.
	Strategy string `gorm:"type:varchar(10)"`
	// Tier and TierPolicyVersion are the tier granted (a tier code or
	// TierNotEligible) and the version of the table it was read from; empty
	// under the engine strategy.
	Tier              string `gorm:"type:varchar(20)"`
	TierPolicyVersion string `gorm:"type:varchar(10)"`
	// EngineQuota is what the individual engine would have granted, kept
	// under every strategy so the tiers can be compared against it. Equal to
	// Quota under the engine strategy.
	EngineQuota decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`

	Applicant string `gorm:"type:text;not null"`
	Result    string `gorm:"type:text;not null"`

	CreatedAt time.Time
}

// assessmentResult is what Result stores: the engine's full result, plus the
// tier decision when the tiers strategy granted the quota. The engine fields
// stay at the top level so rows read the same under either strategy.
type assessmentResult struct {
	LivestockAssessment
	Tier *TierAssessment `json:",omitempty"`
}

// NewLivestockQuotaAssessment records an engine result for its applicant, as
// decided by the engine strategy. ApplyTier switches it to the tiers.
func NewLivestockQuotaAssessment(applicant LivestockApplicant, a LivestockAssessment, currency, requestedBy, reference string) (*LivestockQuotaAssessment, error) {
	in, err := json.Marshal(applicant)
	if err != nil {
		return nil, fmt.Errorf("encoding applicant: %w", err)
	}
	out, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("encoding assessment: %w", err)
	}
	return &LivestockQuotaAssessment{
		ID:            uuid.New(),
		NationalID:    applicant.NationalID,
		Department:    applicant.Department,
		RequestedBy:   requestedBy,
		Reference:     reference,
		Quota:         a.Quota.Quota.Round(2),
		Currency:      currency,
		ActiveCeiling: a.Quota.ActiveCeiling,
		Score:         a.Score.Score,
		Grade:         a.Score.Grade(),
		Typology:      string(a.Typology),
		TermMonths:    a.Quota.TermMonths,
		Strategy:      string(QuotaStrategyEngine),
		EngineQuota:   a.Quota.Quota.Round(2),
		Applicant:     string(in),
		Result:        string(out),
	}, nil
}

// ApplyTier makes the tier the granted quota. The engine's figure stays in
// EngineQuota and in Result, next to the tier decision.
func (r *LivestockQuotaAssessment) ApplyTier(engine LivestockAssessment, t TierAssessment) error {
	out, err := json.Marshal(assessmentResult{LivestockAssessment: engine, Tier: &t})
	if err != nil {
		return fmt.Errorf("encoding assessment: %w", err)
	}
	r.Strategy = string(QuotaStrategyTiers)
	r.Tier = t.Code
	r.TierPolicyVersion = t.PolicyVersion
	r.Quota = t.Amount.Round(2)
	r.ActiveCeiling = t.Label
	r.Result = string(out)
	return nil
}
