package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validProfileInput() model.RiskProfileInput {
	return model.RiskProfileInput{
		MonthlyIncome:        decimal.NewFromInt(500000),
		AdditionalIncome:     decimal.NewFromInt(100000),
		IncomeVerified:       true,
		EmploymentType:       model.EmploymentEmployee,
		EmployerName:         "Acme",
		MonthsEmployed:       24,
		MonthlyExpenses:      decimal.NewFromInt(150000),
		ExistingDebtPayments: decimal.NewFromInt(60000),
		Dependents:           2,
		HousingStatus:        model.HousingRented,
		HasBankAccount:       true,
		AdverseRecords:       0,
	}
}

func TestNewRiskProfile(t *testing.T) {
	clientID := uuid.New()

	tests := []struct {
		name    string
		mutate  func(*model.RiskProfileInput)
		wantErr string
	}{
		{name: "valid profile"},
		{
			name:   "zero income is allowed",
			mutate: func(in *model.RiskProfileInput) { in.MonthlyIncome = decimal.Zero },
		},
		{
			name:    "negative income fails",
			mutate:  func(in *model.RiskProfileInput) { in.MonthlyIncome = decimal.NewFromInt(-1) },
			wantErr: "monthly income cannot be negative",
		},
		{
			name:    "negative additional income fails",
			mutate:  func(in *model.RiskProfileInput) { in.AdditionalIncome = decimal.NewFromInt(-1) },
			wantErr: "additional income cannot be negative",
		},
		{
			name:    "negative expenses fail",
			mutate:  func(in *model.RiskProfileInput) { in.MonthlyExpenses = decimal.NewFromInt(-5) },
			wantErr: "monthly expenses cannot be negative",
		},
		{
			name:    "negative debt payments fail",
			mutate:  func(in *model.RiskProfileInput) { in.ExistingDebtPayments = decimal.NewFromInt(-5) },
			wantErr: "existing debt payments cannot be negative",
		},
		{
			name:    "negative tenure fails",
			mutate:  func(in *model.RiskProfileInput) { in.MonthsEmployed = -1 },
			wantErr: "months employed cannot be negative",
		},
		{
			name:    "negative dependents fail",
			mutate:  func(in *model.RiskProfileInput) { in.Dependents = -1 },
			wantErr: "dependents cannot be negative",
		},
		{
			name:    "unknown employment type fails",
			mutate:  func(in *model.RiskProfileInput) { in.EmploymentType = "astronaut" },
			wantErr: "invalid employment type",
		},
		{
			name:    "unknown housing status fails",
			mutate:  func(in *model.RiskProfileInput) { in.HousingStatus = "houseboat" },
			wantErr: "invalid housing status",
		},
		{
			name: "bureau score above the scale fails",
			mutate: func(in *model.RiskProfileInput) {
				score := model.BureauScoreScale + 1
				in.BureauScore = &score
			},
			wantErr: "bureau score must be between",
		},
		{
			name: "negative bureau score fails",
			mutate: func(in *model.RiskProfileInput) {
				score := -1
				in.BureauScore = &score
			},
			wantErr: "bureau score must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validProfileInput()
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			profile, err := model.NewRiskProfile(clientID, in, model.RiskSourceClient, nil)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, profile)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, profile)
			assert.NotEqual(t, uuid.Nil, profile.ID)
			assert.Equal(t, clientID, profile.ClientID)
			assert.Equal(t, model.RiskSourceClient, profile.Source)
		})
	}
}

func TestRiskProfileUpdateRollsBackOnInvalidInput(t *testing.T) {
	profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
	require.NoError(t, err)

	original := profile.MonthlyIncome
	bad := validProfileInput()
	bad.MonthlyIncome = decimal.NewFromInt(-100)

	err = profile.Update(bad, model.RiskSourceAdmin, nil)

	require.Error(t, err)
	assert.True(t, profile.MonthlyIncome.Equal(original),
		"a rejected update must leave the profile untouched")
	assert.Equal(t, model.RiskSourceClient, profile.Source,
		"provenance must not change when the update is rejected")
}

func TestRiskProfileUpdateAppliesValidInput(t *testing.T) {
	profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
	require.NoError(t, err)

	actor := uuid.New()
	in := validProfileInput()
	in.MonthlyIncome = decimal.NewFromInt(900000)
	in.IncomeVerified = false

	require.NoError(t, profile.Update(in, model.RiskSourceAdmin, &actor))

	assert.True(t, profile.MonthlyIncome.Equal(decimal.NewFromInt(900000)))
	assert.False(t, profile.IncomeVerified)
	assert.Equal(t, model.RiskSourceAdmin, profile.Source)
	require.NotNil(t, profile.SubmittedBy)
	assert.Equal(t, actor, *profile.SubmittedBy)
}

func TestRiskProfileGrossMonthlyIncome(t *testing.T) {
	profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
	require.NoError(t, err)

	assert.True(t, profile.GrossMonthlyIncome().Equal(decimal.NewFromInt(600000)))
}

func TestRiskProfileRecognizedIncome(t *testing.T) {
	haircut := decimal.NewFromFloat(0.20)

	t.Run("verified income is not discounted", func(t *testing.T) {
		profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
		require.NoError(t, err)

		assert.True(t, profile.RecognizedIncome(haircut).Equal(decimal.NewFromInt(600000)))
	})

	t.Run("unverified income takes the haircut", func(t *testing.T) {
		in := validProfileInput()
		in.IncomeVerified = false
		profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
		require.NoError(t, err)

		assert.True(t, profile.RecognizedIncome(haircut).Equal(decimal.NewFromInt(480000)),
			"expected 600000 less 20%%, got %s", profile.RecognizedIncome(haircut))
	})

	t.Run("zero haircut leaves unverified income intact", func(t *testing.T) {
		in := validProfileInput()
		in.IncomeVerified = false
		profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
		require.NoError(t, err)

		assert.True(t, profile.RecognizedIncome(decimal.Zero).Equal(decimal.NewFromInt(600000)))
	})
}

func TestRiskProfileRatios(t *testing.T) {
	profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
	require.NoError(t, err)

	// 60000 / 600000
	assert.True(t, profile.DebtToIncomeRatio().Equal(decimal.NewFromFloat(0.10)),
		"got %s", profile.DebtToIncomeRatio())
	// (150000 + 60000) / 600000
	assert.True(t, profile.ExpenseRatio().Equal(decimal.NewFromFloat(0.35)),
		"got %s", profile.ExpenseRatio())
	assert.True(t, profile.TotalMonthlyObligations().Equal(decimal.NewFromInt(210000)))
}

func TestRiskProfileRatiosWithZeroIncome(t *testing.T) {
	in := validProfileInput()
	in.MonthlyIncome = decimal.Zero
	in.AdditionalIncome = decimal.Zero
	profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
	require.NoError(t, err)

	// Undefined ratios must not panic or divide by zero.
	assert.True(t, profile.DebtToIncomeRatio().IsZero())
	assert.True(t, profile.ExpenseRatio().IsZero())
}

func TestRiskProfileDisposableIncome(t *testing.T) {
	haircut := decimal.NewFromFloat(0.20)
	perDependent := decimal.NewFromInt(50000)

	t.Run("surplus after obligations and dependents", func(t *testing.T) {
		profile, err := model.NewRiskProfile(uuid.New(), validProfileInput(), model.RiskSourceClient, nil)
		require.NoError(t, err)

		// 600000 - 210000 - (2 * 50000)
		assert.True(t, profile.DisposableIncome(haircut, perDependent).Equal(decimal.NewFromInt(290000)),
			"got %s", profile.DisposableIncome(haircut, perDependent))
	})

	t.Run("negative surplus floors at zero", func(t *testing.T) {
		in := validProfileInput()
		in.MonthlyExpenses = decimal.NewFromInt(900000)
		profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
		require.NoError(t, err)

		assert.True(t, profile.DisposableIncome(haircut, perDependent).IsZero(),
			"a client spending more than they earn has no payment capacity")
	})
}

func TestRiskProfileHasBureauScore(t *testing.T) {
	in := validProfileInput()
	profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
	require.NoError(t, err)
	assert.False(t, profile.HasBureauScore())

	score := 700
	in.BureauScore = &score
	withScore, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
	require.NoError(t, err)
	assert.True(t, withScore.HasBureauScore())
}

func TestRiskProfileEmploymentYears(t *testing.T) {
	in := validProfileInput()
	in.MonthsEmployed = 30
	profile, err := model.NewRiskProfile(uuid.New(), in, model.RiskSourceClient, nil)
	require.NoError(t, err)

	assert.True(t, profile.EmploymentYears().Equal(decimal.NewFromFloat(2.5)))
}

func TestRiskProfileInputRestrictToSource(t *testing.T) {
	verifiedScore := 800
	storedScore := 640
	existing := &model.RiskProfile{
		IncomeVerified: true,
		BureauScore:    &storedScore,
		AdverseRecords: 2,
	}
	submitted := model.RiskProfileInput{
		IncomeVerified: true,
		BureauScore:    &verifiedScore,
		AdverseRecords: 0,
	}

	t.Run("admin may set underwriter fields", func(t *testing.T) {
		got := submitted.RestrictToSource(model.RiskSourceAdmin, existing)

		assert.True(t, got.IncomeVerified)
		require.NotNil(t, got.BureauScore)
		assert.Equal(t, 800, *got.BureauScore)
		assert.Equal(t, 0, got.AdverseRecords)
	})

	t.Run("api integrations may set underwriter fields", func(t *testing.T) {
		got := submitted.RestrictToSource(model.RiskSourceAPI, existing)

		require.NotNil(t, got.BureauScore)
		assert.Equal(t, 800, *got.BureauScore)
	})

	for _, source := range []model.RiskDataSource{model.RiskSourceClient, model.RiskSourceVendor} {
		t.Run("stored values survive a "+string(source)+" update", func(t *testing.T) {
			got := submitted.RestrictToSource(source, existing)

			assert.True(t, got.IncomeVerified, "the stored attestation is carried forward")
			require.NotNil(t, got.BureauScore)
			assert.Equal(t, 640, *got.BureauScore,
				"a self-service update must not overwrite bureau data")
			assert.Equal(t, 2, got.AdverseRecords,
				"a self-service update must not clear adverse records")
		})

		t.Run("a first "+string(source)+" submission cannot self-attest", func(t *testing.T) {
			got := submitted.RestrictToSource(source, nil)

			assert.False(t, got.IncomeVerified,
				"income starts unverified until the lender verifies it")
			assert.Nil(t, got.BureauScore)
			assert.Equal(t, 0, got.AdverseRecords)
		})
	}
}
