package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultPolicy(t *testing.T) *model.LivestockQuotaPolicy {
	t.Helper()
	p, err := model.DefaultLivestockQuotaPolicy(nil)
	require.NoError(t, err)
	return p
}

func TestDefaultPolicyRoundTripsTheShippedParameters(t *testing.T) {
	p := defaultPolicy(t)

	// Storing the defaults and reading them back must give the engine exactly
	// what it had hardcoded: the stored policy is the same policy.
	got := p.Params()
	want := model.DefaultQuotaParams()

	assert.True(t, got.MaxQuota.Equal(want.MaxQuota))
	assert.True(t, got.AnnualRatePct.Equal(want.AnnualRatePct))
	assert.True(t, got.LTVPercent.Equal(want.LTVPercent))
	assert.True(t, got.MilkingShare.Equal(want.MilkingShare))
	assert.True(t, got.HectaresPerUGG.Equal(want.HectaresPerUGG))
	assert.Equal(t, want.TermMonths, got.TermMonths)
	assert.Equal(t, want.ApplyInputCeiling, got.ApplyInputCeiling)
	assert.Equal(t, want.LabourRegime, got.LabourRegime)
	assert.Len(t, got.UGGFactors, len(model.OrderedTypologies))

	for _, typology := range model.OrderedTypologies {
		assert.True(t, got.FactorsFor(typology).Breeding.Equal(want.FactorsFor(typology).Breeding),
			"breeding factor for %s", typology)
	}

	// And the engine must reproduce the same quota from the stored parameters.
	applicant := model.LivestockApplicant{
		Department: "Córdoba", Tenure: model.TenureOwned, HoldingStatus: model.HoldingActive,
		SeniorityCycles: 12, CompletedCycles: 12, ReportedCycles: 12,
		Herd: model.Herd{Females2To3Years: 4, Females3To5Years: 3, Males2To3Years: 5, CalvesUnder1Year: 3},
	}
	yields := model.DefaultLivestockYields()
	fromDefaults := model.AssessQuota(applicant, want, yields)
	fromPolicy := model.AssessQuota(applicant, got, yields)
	assert.True(t, fromPolicy.Quota.Quota.Equal(fromDefaults.Quota.Quota),
		"stored %s vs default %s", fromPolicy.Quota.Quota, fromDefaults.Quota.Quota)
}

func TestPolicyValidationRejectsNonsense(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(p *model.QuotaParams)
	}{
		{"zero term", func(p *model.QuotaParams) { p.TermMonths = 0 }},
		{"absurd term", func(p *model.QuotaParams) { p.TermMonths = 360 }},
		{"negative LTV", func(p *model.QuotaParams) { p.LTVPercent = decimal.NewFromInt(-1) }},
		{"negative rate", func(p *model.QuotaParams) { p.AnnualRatePct = decimal.NewFromInt(-5) }},
		{"negative capital cost", func(p *model.QuotaParams) { p.CapitalCostPct = decimal.NewFromInt(-1) }},
		{"zero max quota", func(p *model.QuotaParams) { p.MaxQuota = decimal.Zero }},
		{"negative max quota", func(p *model.QuotaParams) { p.MaxQuota = decimal.NewFromInt(-100) }},
		{"negative price", func(p *model.QuotaParams) { p.PriceCalfPerKg = decimal.NewFromInt(-1) }},
		{"milking share above one", func(p *model.QuotaParams) { p.MilkingShare = decimal.NewFromFloat(1.5) }},
		{"negative daily wage", func(p *model.QuotaParams) { p.DailyWage = decimal.NewFromInt(-1) }},
		{"negative headroom", func(p *model.QuotaParams) { p.InputHeadroom = decimal.NewFromFloat(-0.5) }},
		{"impossible occasional days", func(p *model.QuotaParams) { p.OccasionalDaysPerMonth = 45 }},
		{"unknown labour regime", func(p *model.QuotaParams) { p.LabourRegime = "whatever" }},
		{
			name: "labour thresholds out of order",
			break_: func(p *model.QuotaParams) {
				p.UGGThresholdOccasional = decimal.NewFromInt(10) // below the family one
			},
		},
		{
			name: "equal labour thresholds",
			break_: func(p *model.QuotaParams) {
				p.UGGThresholdPartial = p.UGGThresholdOccasional
			},
		},
		{
			name: "negative UGG factor",
			break_: func(p *model.QuotaParams) {
				f := p.UGGFactors[model.TypologyDualPurpose]
				f.Breeding = decimal.NewFromInt(-1)
				p.UGGFactors[model.TypologyDualPurpose] = f
			},
		},
		{
			name: "missing typology in the matrix",
			break_: func(p *model.QuotaParams) {
				delete(p.UGGFactors, model.TypologyFattening)
			},
		},
		{
			name: "unknown typology in the matrix",
			break_: func(p *model.QuotaParams) {
				p.UGGFactors["Pastoreo lunar"] = model.UGGFactors{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := model.DefaultQuotaParams()
			tt.break_(&params)
			_, err := model.NewLivestockQuotaPolicy("CuppoGan", "", params, nil)
			assert.Error(t, err, "a bad parameter must be rejected, not stored")
		})
	}

	t.Run("name is required", func(t *testing.T) {
		_, err := model.NewLivestockQuotaPolicy("  ", "", model.DefaultQuotaParams(), nil)
		assert.Error(t, err)
	})
}

func TestUpdateParamsRollsBackOnInvalidInput(t *testing.T) {
	p := defaultPolicy(t)
	before := p.Params()

	broken := model.DefaultQuotaParams()
	broken.MaxQuota = decimal.Zero
	require.Error(t, p.UpdateParams(broken))

	// A rejected edit must leave the policy exactly as it was.
	assert.True(t, p.Params().MaxQuota.Equal(before.MaxQuota))

	valid := model.DefaultQuotaParams()
	valid.MaxQuota = decimal.NewFromInt(5000000)
	require.NoError(t, p.UpdateParams(valid))
	assert.True(t, p.MaxQuota.Equal(decimal.NewFromInt(5000000)))
}

func TestNextVersionLeavesTheRunningVersionUntouched(t *testing.T) {
	p := defaultPolicy(t)
	p.Activate()

	params := model.DefaultQuotaParams()
	params.AnnualRatePct = decimal.NewFromInt(30)
	next, err := p.NextVersion("Rate moved to 30% E.A.", params, nil)
	require.NoError(t, err)

	assert.Equal(t, p.Version+1, next.Version)
	assert.Equal(t, p.Name, next.Name)
	assert.False(t, next.IsActive, "a new version must not activate itself")
	assert.True(t, p.IsActive, "the running version keeps serving")
	assert.True(t, p.AnnualRatePct.Equal(decimal.NewFromInt(26)), "the source version is untouched")
	assert.True(t, next.AnnualRatePct.Equal(decimal.NewFromInt(30)))
	assert.NotEqual(t, p.ID, next.ID)

	next.Activate()
	assert.True(t, next.IsActive)
	next.Deactivate()
	assert.False(t, next.IsActive)
}

func TestEditedFactorsReachTheEngine(t *testing.T) {
	// The matrix is the point of the exercise: halving the breeding factor must
	// move the UGG, hence the costs, hence the inputs ceiling.
	params := model.DefaultQuotaParams()
	herd := model.Herd{Females2To3Years: 10, Females3To5Years: 10, Females1To2Years: 2}
	classification := herd.Classify()
	typology := classification.Typology()

	base := classification.UGGWith(params.FactorsFor(typology))

	f := params.UGGFactors[typology]
	f.Breeding = f.Breeding.Div(decimal.NewFromInt(2))
	params.UGGFactors[typology] = f

	assert.True(t, classification.UGGWith(params.FactorsFor(typology)).LessThan(base),
		"halving the breeding factor must lower the herd's UGG")

	// An empty matrix falls back to the shipped factors rather than zeroing the
	// herd, so parameters built without a matrix still behave.
	bare := model.QuotaParams{}
	assert.True(t, bare.FactorsFor(typology).Breeding.Equal(model.FactorsFor(typology).Breeding))
}
