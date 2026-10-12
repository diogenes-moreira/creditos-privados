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

	Applicant string `gorm:"type:text;not null"`
	Result    string `gorm:"type:text;not null"`

	CreatedAt time.Time
}

// NewLivestockQuotaAssessment records an engine result for its applicant.
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
		Applicant:     string(in),
		Result:        string(out),
	}, nil
}
