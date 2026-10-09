package model_test

import (
	"strings"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPrompt = "Assess this applicant.\n{{profile}}\n{{policy}}"

func testParams() model.RiskModelParams {
	return model.DefaultRiskModelParams("AR")
}

func newTestModel(t *testing.T) *model.RiskModel {
	t.Helper()
	m, err := model.NewRiskModel("Test model", testPrompt, "",
		testParams(), model.DefaultRiskGradeBands(), nil)
	require.NoError(t, err)
	return m
}

func TestNewRiskModel(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*model.RiskModelParams)
		prompt  string
		bands   []model.RiskGradeBand
		wantErr string
	}{
		{name: "valid model", prompt: testPrompt, bands: model.DefaultRiskGradeBands()},
		{
			name: "empty prompt fails", prompt: "", bands: model.DefaultRiskGradeBands(),
			wantErr: "analysis prompt is required",
		},
		{
			name:    "prompt without the profile placeholder fails",
			prompt:  "Assess the applicant using {{policy}} only.",
			bands:   model.DefaultRiskGradeBands(),
			wantErr: "must include the {{profile}} placeholder",
		},
		{
			name:    "unknown placeholder is rejected",
			prompt:  "{{profile}} and {{credit_score}}",
			bands:   model.DefaultRiskGradeBands(),
			wantErr: "unknown prompt placeholders: credit_score",
		},
		{
			name: "no bands fails", prompt: testPrompt, bands: nil,
			wantErr: "at least one grade band is required",
		},
		{
			name:   "bands that do not reach zero fail",
			prompt: testPrompt,
			bands: []model.RiskGradeBand{{
				Grade: model.RiskGradeA, MinScore: 50,
				ExposureFactor: decimal.NewFromInt(1), Decision: model.RiskDecisionApprove,
			}},
			wantErr: "lowest grade band must start at score 0",
		},
		{
			name:   "duplicate grade fails",
			prompt: testPrompt,
			bands: []model.RiskGradeBand{
				{Grade: model.RiskGradeA, MinScore: 0, ExposureFactor: decimal.NewFromInt(1), Decision: model.RiskDecisionApprove},
				{Grade: model.RiskGradeA, MinScore: 50, ExposureFactor: decimal.NewFromInt(1), Decision: model.RiskDecisionApprove},
			},
			wantErr: "duplicate grade band",
		},
		{
			name:   "exposure factor above one fails",
			prompt: testPrompt,
			bands: []model.RiskGradeBand{{
				Grade: model.RiskGradeA, MinScore: 0,
				ExposureFactor: decimal.NewFromFloat(1.5), Decision: model.RiskDecisionApprove,
			}},
			wantErr: "exposure factor must be a fraction between 0 and 1",
		},
		{
			name:   "invalid band decision fails",
			prompt: testPrompt,
			bands: []model.RiskGradeBand{{
				Grade: model.RiskGradeA, MinScore: 0,
				ExposureFactor: decimal.NewFromInt(1), Decision: "maybe",
			}},
			wantErr: "invalid decision",
		},
		{
			name:    "age below 18 fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.MinAge = 16 },
			wantErr: "minimum age cannot be below 18",
		},
		{
			name:    "max age below min age fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.MaxAge = 18 },
			wantErr: "maximum age must be greater than minimum age",
		},
		{
			name:    "DTI above one fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.MaxDTI = decimal.NewFromFloat(1.2) },
			wantErr: "max DTI must be a fraction between 0 and 1",
		},
		{
			name:    "zero payment-to-income cap fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.MaxPaymentToIncome = decimal.Zero },
			wantErr: "max payment to income must be positive",
		},
		{
			name:    "non-positive exposure ceiling fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.AbsoluteMaxLimit = decimal.Zero },
			wantErr: "absolute max limit must be positive",
		},
		{
			name:    "term beyond 60 months fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.MaxInstallments = 72 },
			wantErr: "max installments must be between 1 and 60",
		},
		{
			name:    "negative base rate fails",
			prompt:  testPrompt,
			bands:   model.DefaultRiskGradeBands(),
			mutate:  func(p *model.RiskModelParams) { p.BaseInterestRate = decimal.NewFromInt(-1) },
			wantErr: "base interest rate cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testParams()
			if tt.mutate != nil {
				tt.mutate(&params)
			}
			m, err := model.NewRiskModel("Test model", tt.prompt, "", params, tt.bands, nil)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, m.Version)
			assert.False(t, m.IsActive, "a new model must start inactive")
		})
	}
}

func TestRiskModelBandsAreSortedByScoreDescending(t *testing.T) {
	m := newTestModel(t)

	require.NotEmpty(t, m.Bands)
	for i := 1; i < len(m.Bands); i++ {
		assert.GreaterOrEqual(t, m.Bands[i-1].MinScore, m.Bands[i].MinScore)
	}
}

func TestRiskModelRenderPrompt(t *testing.T) {
	m, err := model.NewRiskModel("Test", "Client: {{client}} Data: {{profile}} Missing: {{history}}",
		"", testParams(), model.DefaultRiskGradeBands(), nil)
	require.NoError(t, err)

	rendered := m.RenderPrompt(map[string]string{
		"client":  "Jane Doe",
		"profile": "income 500000",
	})

	assert.Contains(t, rendered, "Client: Jane Doe")
	assert.Contains(t, rendered, "Data: income 500000")
	assert.Contains(t, rendered, "Missing: ",
		"a placeholder with no value renders empty rather than literally")
	assert.NotContains(t, rendered, "{{")
}

func TestRiskModelRenderPromptToleratesWhitespaceInPlaceholders(t *testing.T) {
	m, err := model.NewRiskModel("Test", "Data: {{ profile }}", "",
		testParams(), model.DefaultRiskGradeBands(), nil)
	require.NoError(t, err)

	assert.Equal(t, "Data: X", m.RenderPrompt(map[string]string{"profile": "X"}))
}

func TestUnknownPlaceholders(t *testing.T) {
	unknown := model.UnknownPlaceholders("{{profile}} {{bogus}} {{policy}} {{bogus}} {{other}}")

	assert.ElementsMatch(t, []string{"bogus", "other"}, unknown,
		"each unknown placeholder is reported once")
	assert.Empty(t, model.UnknownPlaceholders("{{profile}} {{client}} {{today}}"))
}

func TestRiskModelBandFor(t *testing.T) {
	m := newTestModel(t)

	tests := []struct {
		score int
		want  model.RiskGrade
	}{
		{100, model.RiskGradeA},
		{80, model.RiskGradeA},
		{79, model.RiskGradeB},
		{65, model.RiskGradeB},
		{64, model.RiskGradeC},
		{50, model.RiskGradeC},
		{49, model.RiskGradeD},
		{35, model.RiskGradeD},
		{34, model.RiskGradeE},
		{0, model.RiskGradeE},
	}

	for _, tt := range tests {
		band := m.BandFor(tt.score)
		require.NotNil(t, band, "score %d must map to a band", tt.score)
		assert.Equal(t, tt.want, band.Grade, "score %d", tt.score)
	}
}

func TestRiskModelRateFor(t *testing.T) {
	m := newTestModel(t)

	bandA := m.BandForGrade(model.RiskGradeA)
	require.NotNil(t, bandA)
	assert.True(t, m.RateFor(bandA).Equal(m.BaseInterestRate),
		"grade A carries no spread")

	bandD := m.BandForGrade(model.RiskGradeD)
	require.NotNil(t, bandD)
	assert.True(t, m.RateFor(bandD).Equal(m.BaseInterestRate.Add(bandD.InterestSpread)))

	assert.True(t, m.RateFor(nil).Equal(m.BaseInterestRate),
		"an unmatched band falls back to the base rate")
}

func TestRiskModelTermCapFor(t *testing.T) {
	m := newTestModel(t)

	bandE := m.BandForGrade(model.RiskGradeE)
	require.NotNil(t, bandE)
	assert.Equal(t, 3, m.TermCapFor(bandE), "the band cap wins when it is tighter")

	assert.Equal(t, m.MaxInstallments, m.TermCapFor(nil))

	loose := model.RiskGradeBand{MaxInstallments: 999}
	assert.Equal(t, m.MaxInstallments, m.TermCapFor(&loose),
		"a band cannot widen the model-level term cap")

	unset := model.RiskGradeBand{MaxInstallments: 0}
	assert.Equal(t, m.MaxInstallments, m.TermCapFor(&unset))
}

func TestRiskModelRoundLimit(t *testing.T) {
	m := newTestModel(t)
	// AR defaults: rounding step 10000, ceiling 5000000.

	assert.True(t, m.RoundLimit(decimal.NewFromInt(123456)).Equal(decimal.NewFromInt(120000)),
		"limits round down to the step")
	assert.True(t, m.RoundLimit(decimal.NewFromInt(9999)).IsZero(),
		"an amount below one step rounds to zero")
	assert.True(t, m.RoundLimit(decimal.NewFromInt(99999999)).Equal(m.AbsoluteMaxLimit),
		"the absolute ceiling caps the limit")
	assert.True(t, m.RoundLimit(decimal.NewFromInt(-500)).IsZero(),
		"a negative amount yields no limit")
}

func TestRiskModelNextVersion(t *testing.T) {
	m := newTestModel(t)
	m.Activate()
	actor := uuid.New()

	next, err := m.NextVersion("Updated analysis. {{profile}}", "second take",
		testParams(), nil, &actor)
	require.NoError(t, err)

	assert.Equal(t, 2, next.Version)
	assert.Equal(t, m.Name, next.Name, "a version keeps the model name")
	assert.Contains(t, next.Prompt, "Updated analysis")
	assert.False(t, next.IsActive, "a new version must not activate itself")
	assert.True(t, m.IsActive, "the running version keeps serving")
	assert.Len(t, next.Bands, len(m.Bands), "bands carry over when none are supplied")
	assert.NotEqual(t, m.ID, next.ID)

	for _, b := range next.Bands {
		assert.Equal(t, next.ID, b.RiskModelID, "cloned bands belong to the new version")
		assert.NotEqual(t, uuid.Nil, b.ID)
	}
}

func TestRiskModelNextVersionRejectsInvalidPrompt(t *testing.T) {
	m := newTestModel(t)

	next, err := m.NextVersion("No placeholders here", "", testParams(), nil, nil)

	require.Error(t, err)
	assert.Nil(t, next)
}

func TestRiskModelUpdatePromptRollsBackOnFailure(t *testing.T) {
	m := newTestModel(t)
	original := m.Prompt

	require.Error(t, m.UpdatePrompt("missing the required placeholder"))
	assert.Equal(t, original, m.Prompt)

	require.NoError(t, m.UpdatePrompt("New prompt {{profile}}"))
	assert.Equal(t, "New prompt {{profile}}", m.Prompt)
}

func TestRiskModelUpdateGuardrailsRollsBackOnFailure(t *testing.T) {
	m := newTestModel(t)
	original := m.MaxDTI

	bad := testParams()
	bad.MaxDTI = decimal.NewFromInt(5)

	require.Error(t, m.UpdateGuardrails(bad))
	assert.True(t, m.MaxDTI.Equal(original), "a rejected edit leaves policy untouched")
}

func TestRiskModelActivateDeactivate(t *testing.T) {
	m := newTestModel(t)

	assert.False(t, m.IsActive)
	m.Activate()
	assert.True(t, m.IsActive)
	m.Deactivate()
	assert.False(t, m.IsActive)
}

func TestDefaultRiskModel(t *testing.T) {
	for _, country := range []string{"AR", "CO"} {
		t.Run(country, func(t *testing.T) {
			m, err := model.DefaultRiskModel(country, nil)
			require.NoError(t, err)

			assert.True(t, m.IsActive, "the shipped model is ready to use")
			require.NoError(t, m.Validate())
			assert.Len(t, m.Bands, len(model.OrderedRiskGrades))
			assert.True(t, strings.Contains(m.Prompt, "{{profile}}"))
			assert.Empty(t, model.UnknownPlaceholders(m.Prompt),
				"the shipped prompt only uses known placeholders")
		})
	}
}

func TestStricterOf(t *testing.T) {
	tests := []struct {
		a, b, want model.RiskDecision
	}{
		{model.RiskDecisionApprove, model.RiskDecisionManualReview, model.RiskDecisionManualReview},
		{model.RiskDecisionManualReview, model.RiskDecisionApprove, model.RiskDecisionManualReview},
		{model.RiskDecisionApprove, model.RiskDecisionDecline, model.RiskDecisionDecline},
		{model.RiskDecisionDecline, model.RiskDecisionApprove, model.RiskDecisionDecline},
		{model.RiskDecisionApprove, model.RiskDecisionApprove, model.RiskDecisionApprove},
		{model.RiskDecisionDecline, model.RiskDecisionManualReview, model.RiskDecisionDecline},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, model.StricterOf(tt.a, tt.b), "%s vs %s", tt.a, tt.b)
	}
}
