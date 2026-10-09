package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAssessmentFixtures(t *testing.T) (*model.RiskProfile, *model.RiskModel) {
	t.Helper()
	profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceAdmin, nil)
	require.NoError(t, err)
	return profile, newTestModel(t)
}

func approvedEvaluation() model.RiskEvaluation {
	return model.RiskEvaluation{
		Analyzer:                model.AnalyzerScorecard,
		Score:                   82,
		Grade:                   model.RiskGradeA,
		ProbabilityOfDefault:    decimal.NewFromFloat(0.02),
		Decision:                model.RiskDecisionApprove,
		MaxMonthlyPayment:       decimal.NewFromInt(150000),
		RecommendedLimit:        decimal.NewFromInt(1200000),
		RecommendedInstallments: 24,
		RecommendedInterestRate: decimal.NewFromFloat(0.60),
		Summary:                 "Strong applicant.",
		PolicyNotes:             []string{"first note", "second note"},
		Factors: []model.RiskFactor{
			{Code: "affordability", Label: "Payment capacity", Points: 90, Impact: model.FactorPositive},
			{Code: "bureau", Label: "Bureau score", Points: 85, Impact: model.FactorPositive},
		},
	}
}

func TestNewRiskAssessment(t *testing.T) {
	profile, m := testAssessmentFixtures(t)
	clientID := uuid.New()
	actor := uuid.New()

	a, err := model.NewRiskAssessment(clientID, profile, m, approvedEvaluation(), &actor)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, a.ID)
	assert.Equal(t, clientID, a.ClientID)
	assert.Equal(t, profile.ID, a.RiskProfileID)
	assert.Equal(t, m.ID, a.RiskModelID)
	assert.Equal(t, m.Name, a.RiskModelName)
	assert.Equal(t, m.Version, a.ModelVersion,
		"the assessment records which model version scored it")
	assert.Equal(t, 82, a.Score)
	assert.Equal(t, model.RiskGradeA, a.Grade)
	assert.Equal(t, model.RiskDecisionApprove, a.Decision)
	require.NotNil(t, a.EvaluatedBy)
	assert.Equal(t, actor, *a.EvaluatedBy)
}

func TestNewRiskAssessmentLinksFactors(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	a, err := model.NewRiskAssessment(uuid.New(), profile, m, approvedEvaluation(), nil)
	require.NoError(t, err)

	require.Len(t, a.Factors, 2)
	for _, f := range a.Factors {
		assert.NotEqual(t, uuid.Nil, f.ID)
		assert.Equal(t, a.ID, f.AssessmentID)
		assert.Equal(t, a.CreatedAt, f.CreatedAt)
	}
}

func TestNewRiskAssessmentValidation(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	tests := []struct {
		name    string
		profile *model.RiskProfile
		model   *model.RiskModel
		mutate  func(*model.RiskEvaluation)
		wantErr string
	}{
		{
			name: "missing profile", profile: nil, model: m,
			wantErr: "risk profile is required",
		},
		{
			name: "missing model", profile: profile, model: nil,
			wantErr: "risk model is required",
		},
		{
			name: "score above 100", profile: profile, model: m,
			mutate:  func(e *model.RiskEvaluation) { e.Score = 101 },
			wantErr: "score must be between 0 and 100",
		},
		{
			name: "negative score", profile: profile, model: m,
			mutate:  func(e *model.RiskEvaluation) { e.Score = -1 },
			wantErr: "score must be between 0 and 100",
		},
		{
			name: "invalid decision", profile: profile, model: m,
			mutate:  func(e *model.RiskEvaluation) { e.Decision = "perhaps" },
			wantErr: "invalid decision",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := approvedEvaluation()
			if tt.mutate != nil {
				tt.mutate(&eval)
			}
			a, err := model.NewRiskAssessment(uuid.New(), tt.profile, tt.model, eval, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, a)
		})
	}
}

func TestRiskAssessmentDecisionHelpers(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	approved, err := model.NewRiskAssessment(uuid.New(), profile, m, approvedEvaluation(), nil)
	require.NoError(t, err)
	assert.True(t, approved.IsApproved())
	assert.False(t, approved.RequiresReview())

	eval := approvedEvaluation()
	eval.Decision = model.RiskDecisionManualReview
	review, err := model.NewRiskAssessment(uuid.New(), profile, m, eval, nil)
	require.NoError(t, err)
	assert.False(t, review.IsApproved())
	assert.True(t, review.RequiresReview())
}

func TestRiskAssessmentAvailableCredit(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	approved, err := model.NewRiskAssessment(uuid.New(), profile, m, approvedEvaluation(), nil)
	require.NoError(t, err)

	tests := []struct {
		name string
		used decimal.Decimal
		want decimal.Decimal
	}{
		{"nothing drawn", decimal.Zero, decimal.NewFromInt(1200000)},
		{"partially drawn", decimal.NewFromInt(200000), decimal.NewFromInt(1000000)},
		{"fully drawn", decimal.NewFromInt(1200000), decimal.Zero},
		{"over-drawn floors at zero", decimal.NewFromInt(1500000), decimal.Zero},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := approved.AvailableCredit(tt.used)
			assert.True(t, got.Equal(tt.want), "want %s, got %s", tt.want, got)
		})
	}
}

func TestRiskAssessmentDeclinedHasNoAvailability(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	eval := approvedEvaluation()
	eval.Decision = model.RiskDecisionDecline
	declined, err := model.NewRiskAssessment(uuid.New(), profile, m, eval, nil)
	require.NoError(t, err)

	assert.True(t, declined.AvailableCredit(decimal.Zero).IsZero(),
		"a declined assessment offers no credit even with a stored limit")
}

func TestRiskAssessmentNotesRoundTrip(t *testing.T) {
	profile, m := testAssessmentFixtures(t)

	a, err := model.NewRiskAssessment(uuid.New(), profile, m, approvedEvaluation(), nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"first note", "second note"}, a.Notes())

	eval := approvedEvaluation()
	eval.PolicyNotes = nil
	empty, err := model.NewRiskAssessment(uuid.New(), profile, m, eval, nil)
	require.NoError(t, err)
	assert.Nil(t, empty.Notes())
}
