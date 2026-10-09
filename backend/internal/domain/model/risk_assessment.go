package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Analyzer names identify which engine produced an assessment.
const (
	AnalyzerScorecard = "scorecard"
	AnalyzerLLM       = "llm"
)

// FactorImpact describes how a factor moved the overall score.
type FactorImpact string

const (
	FactorPositive FactorImpact = "positive"
	FactorNegative FactorImpact = "negative"
	FactorNeutral  FactorImpact = "neutral"
)

// RiskAssessment is the persisted outcome of running the analysis model over a
// client's risk profile. It is the output side of the risk module and carries
// the credit availability the client qualifies for.
type RiskAssessment struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	ClientID      uuid.UUID `gorm:"type:uuid;index;not null"`
	Client        Client    `gorm:"foreignKey:ClientID"`
	RiskProfileID uuid.UUID `gorm:"type:uuid;index;not null"`
	RiskModelID   uuid.UUID `gorm:"type:uuid;index;not null"`
	RiskModelName string    `gorm:"not null"`
	ModelVersion  int       `gorm:"not null"`
	Analyzer      string    `gorm:"type:varchar(20);not null"`

	Score                int             `gorm:"not null"`
	Grade                RiskGrade       `gorm:"type:varchar(2);not null"`
	ProbabilityOfDefault decimal.Decimal `gorm:"type:decimal(6,4);not null;default:0"`
	Decision             RiskDecision    `gorm:"type:varchar(20);not null"`

	MaxMonthlyPayment       decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	RecommendedLimit        decimal.Decimal `gorm:"type:decimal(18,2);not null;default:0"`
	RecommendedInstallments int             `gorm:"not null;default:0"`
	RecommendedInterestRate decimal.Decimal `gorm:"type:decimal(8,4);not null;default:0"`

	Summary     string `gorm:"type:text"`
	PolicyNotes string `gorm:"type:text"`
	RawResponse string `gorm:"type:text"`

	Factors []RiskFactor `gorm:"foreignKey:AssessmentID"`

	EvaluatedBy *uuid.UUID `gorm:"type:uuid"`
	CreatedAt   time.Time  `gorm:"not null;index"`
}

// RiskFactor is a single explanatory driver of an assessment's score.
type RiskFactor struct {
	ID           uuid.UUID       `gorm:"type:uuid;primaryKey"`
	AssessmentID uuid.UUID       `gorm:"type:uuid;index;not null"`
	Code         string          `gorm:"not null"`
	Label        string          `gorm:"not null"`
	Value        string          `gorm:""`
	Impact       FactorImpact    `gorm:"type:varchar(10);not null;default:'neutral'"`
	Weight       decimal.Decimal `gorm:"type:decimal(6,4);not null;default:0"`
	Points       int             `gorm:"not null;default:0"`
	Comment      string          `gorm:"type:text"`
	CreatedAt    time.Time       `gorm:"not null"`
}

// RiskEvaluation is the in-memory result of an analysis, before persistence.
// Both the deterministic scorecard and the prompt-driven analyzer produce one.
type RiskEvaluation struct {
	Score                   int
	Grade                   RiskGrade
	ProbabilityOfDefault    decimal.Decimal
	Decision                RiskDecision
	MaxMonthlyPayment       decimal.Decimal
	RecommendedLimit        decimal.Decimal
	RecommendedInstallments int
	RecommendedInterestRate decimal.Decimal
	Summary                 string
	Factors                 []RiskFactor
	PolicyNotes             []string
	Analyzer                string
	RawResponse             string
}

// AddNote records a policy note, skipping duplicates.
func (e *RiskEvaluation) AddNote(note string) {
	for _, existing := range e.PolicyNotes {
		if existing == note {
			return
		}
	}
	e.PolicyNotes = append(e.PolicyNotes, note)
}

// NewRiskAssessment turns an evaluation into a persistable assessment.
func NewRiskAssessment(clientID uuid.UUID, profile *RiskProfile, m *RiskModel, eval RiskEvaluation, evaluatedBy *uuid.UUID) (*RiskAssessment, error) {
	if profile == nil {
		return nil, fmt.Errorf("risk profile is required")
	}
	if m == nil {
		return nil, fmt.Errorf("risk model is required")
	}
	if eval.Score < 0 || eval.Score > 100 {
		return nil, fmt.Errorf("score must be between 0 and 100, got %d", eval.Score)
	}
	if !eval.Decision.valid() {
		return nil, fmt.Errorf("invalid decision: %s", eval.Decision)
	}

	a := &RiskAssessment{
		ID:                      uuid.New(),
		ClientID:                clientID,
		RiskProfileID:           profile.ID,
		RiskModelID:             m.ID,
		RiskModelName:           m.Name,
		ModelVersion:            m.Version,
		Analyzer:                eval.Analyzer,
		Score:                   eval.Score,
		Grade:                   eval.Grade,
		ProbabilityOfDefault:    eval.ProbabilityOfDefault,
		Decision:                eval.Decision,
		MaxMonthlyPayment:       eval.MaxMonthlyPayment,
		RecommendedLimit:        eval.RecommendedLimit,
		RecommendedInstallments: eval.RecommendedInstallments,
		RecommendedInterestRate: eval.RecommendedInterestRate,
		Summary:                 eval.Summary,
		PolicyNotes:             strings.Join(eval.PolicyNotes, "\n"),
		RawResponse:             eval.RawResponse,
		EvaluatedBy:             evaluatedBy,
		CreatedAt:               time.Now(),
	}

	for _, f := range eval.Factors {
		f.ID = uuid.New()
		f.AssessmentID = a.ID
		f.CreatedAt = a.CreatedAt
		a.Factors = append(a.Factors, f)
	}
	return a, nil
}

// IsApproved reports whether the assessment recommends granting credit.
func (a *RiskAssessment) IsApproved() bool {
	return a.Decision == RiskDecisionApprove
}

// RequiresReview reports whether a human must decide on the application.
func (a *RiskAssessment) RequiresReview() bool {
	return a.Decision == RiskDecisionManualReview
}

// AvailableCredit is the headroom left once existing exposure is deducted from
// the recommended limit. A declined assessment carries no availability.
func (a *RiskAssessment) AvailableCredit(usedAmount decimal.Decimal) decimal.Decimal {
	if a.Decision == RiskDecisionDecline {
		return decimal.Zero
	}
	available := a.RecommendedLimit.Sub(usedAmount)
	if available.IsNegative() {
		return decimal.Zero
	}
	return available.Round(2)
}

// Notes splits the stored policy notes back into a list.
func (a *RiskAssessment) Notes() []string {
	if strings.TrimSpace(a.PolicyNotes) == "" {
		return nil
	}
	return strings.Split(a.PolicyNotes, "\n")
}
