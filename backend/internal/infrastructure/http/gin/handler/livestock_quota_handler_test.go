package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServiceKey = "test-cuppogan-key"

type memoryLivestockQuotaRepo struct {
	saved []*model.LivestockQuotaAssessment
}

func (m *memoryLivestockQuotaRepo) Create(_ context.Context, a *model.LivestockQuotaAssessment) error {
	m.saved = append(m.saved, a)
	return nil
}

func (m *memoryLivestockQuotaRepo) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockQuotaAssessment, error) {
	for _, a := range m.saved {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func newLivestockQuotaRouter(repo *memoryLivestockQuotaRepo) *gin.Engine {
	return newLivestockQuotaRouterWith(repo, model.QuotaStrategyTiers)
}

func newLivestockQuotaRouterWith(repo *memoryLivestockQuotaRepo, strategy model.QuotaStrategy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewLivestockQuotaHandler(service.NewLivestockQuotaService(repo, "COP").WithStrategy(strategy))
	r.POST("/livestock-quotas", middleware.RequireServiceKey("cuppogan", testServiceKey), h.Assess)
	return r
}

// holding001 is holding 001 of the engine's reference verification table.
func holding001() map[string]any {
	return map[string]any{
		"nationalId": "123456781", "department": "Córdoba", "reference": "onboarding-1",
		"tenure": "Propia", "holdingStatus": "N",
		"seniorityCycles": 12, "completedCycles": 12, "reportedCycles": 12,
		"holdings": []map[string]any{{
			"femalesUnder3Months": 1, "females3To8Months": 1, "females1To2Years": 1, "females2To3Years": 1,
			"females3To5Years": 1, "femalesOver5Years": 1,
			"males1To2Years": 2, "males2To3Years": 5, "malesOver3Years": 3,
			"notVaccinated": 1,
		}},
	}
}

func post(t *testing.T, r *gin.Engine, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/livestock-quotas", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(middleware.ServiceKeyHeader, key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// holding001Engine is what the engine computes for holding001.
func holding001Engine() model.LivestockAssessment {
	return model.AssessQuota(model.LivestockApplicant{
		NationalID: "123456781", Department: "Córdoba", Tenure: model.TenureOwned, HoldingStatus: model.HoldingActive,
		SeniorityCycles: 12, CompletedCycles: 12, ReportedCycles: 12,
		Herd: model.Herd{
			FemalesUnder3Months: 1, Females3To8Months: 1, Females1To2Years: 1, Females2To3Years: 1,
			Females3To5Years: 1, FemalesOver5Years: 1, Males1To2Years: 2, Males2To3Years: 5, MalesOver3Years: 3,
			NotVaccinated: 1,
		},
	}, model.DefaultQuotaParams(), model.DefaultLivestockYields())
}

func TestLivestockQuotaAssessesAndStoresTheQuota(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	w := post(t, newLivestockQuotaRouterWith(repo, model.QuotaStrategyEngine), testServiceKey, holding001())

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.LivestockQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// The endpoint is the engine: the same herd gives the same quota. (The reference
	// table's figure for holding 001 also counts CalvesUnder1Year, which the RUV
	// does not report, so it is not comparable.)
	want := holding001Engine()
	assert.Equal(t, want.Quota.Quota.StringFixed(2), resp.Quota)
	assert.Equal(t, resp.Quota, resp.EngineQuota)
	assert.Equal(t, "engine", resp.Strategy)
	assert.Nil(t, resp.Tier)
	assert.True(t, decimal.RequireFromString(resp.Quota).IsPositive())
	assert.Equal(t, want.Quota.ActiveCeiling, resp.ActiveCeiling)
	assert.Equal(t, want.Classification.Total, resp.Head)
	assert.Equal(t, "COP", resp.Currency)
	assert.Equal(t, want.Score.Score, resp.Score.Score)
	assert.Equal(t, string(want.Typology), resp.Typology)
	assert.Equal(t, "onboarding-1", resp.Reference)

	require.Len(t, repo.saved, 1)
	saved := repo.saved[0]
	assert.Equal(t, resp.ID, saved.ID.String())
	assert.Equal(t, "cuppogan", saved.RequestedBy)
	assert.Equal(t, "123456781", saved.NationalID)
	assert.Contains(t, saved.Applicant, `"Department":"Córdoba"`)
	assert.Contains(t, saved.Result, `"ActiveCeiling"`)
	assert.Equal(t, "engine", saved.Strategy)
	assert.Empty(t, saved.Tier)
	assert.True(t, saved.EngineQuota.Equal(saved.Quota))
}

func TestLivestockQuotaGrantsTheTierAndKeepsTheEngineQuotaByDefault(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	w := post(t, newLivestockQuotaRouter(repo), testServiceKey, holding001())

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.LivestockQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// 16 vaccinated heads, 1 not vaccinated: 94.12%, enough for tier 1 but
	// short of tier 2's 30 heads.
	want := holding001Engine()
	assert.Equal(t, "tiers", resp.Strategy)
	assert.Equal(t, "500000.00", resp.Quota)
	assert.Equal(t, want.Quota.Quota.StringFixed(2), resp.EngineQuota)
	assert.Equal(t, "Escalón 1", resp.ActiveCeiling)
	assert.Equal(t, "tier1", resp.ActiveCeilingCode)
	assert.Equal(t, want.Quota.LTVCeiling.StringFixed(2), resp.Ceilings.LTV, "the engine's ceilings stay")
	require.NotNil(t, resp.Tier)
	assert.Equal(t, "tier1", resp.Tier.Code)
	assert.Equal(t, "v1", resp.Tier.PolicyVersion)
	assert.Equal(t, 16, resp.Tier.VaccinatedHeads)
	assert.Equal(t, "0.9412", resp.Tier.VaccinationRate)
	assert.Equal(t, []dto.QuotaTierReasonResponse{
		{Scope: "tier2", Requirement: "vaccinatedHeads", Required: ">=30", Actual: "16"},
	}, resp.Tier.Reasons)

	require.Len(t, repo.saved, 1)
	saved := repo.saved[0]
	assert.Equal(t, "tiers", saved.Strategy)
	assert.Equal(t, "tier1", saved.Tier)
	assert.Equal(t, "v1", saved.TierPolicyVersion)
	assert.Equal(t, "500000.00", saved.Quota.StringFixed(2))
	assert.Equal(t, want.Quota.Quota.StringFixed(2), saved.EngineQuota.StringFixed(2))
	assert.Contains(t, saved.Result, `"Tier":{"Code":"tier1"`)
	assert.Contains(t, saved.Result, `"ActiveCeiling"`, "the engine result is still stored")
}

func TestLivestockQuotaSumsTheHoldingsOfOneNationalID(t *testing.T) {
	one := post(t, newLivestockQuotaRouter(&memoryLivestockQuotaRepo{}), testServiceKey, holding001())
	body := holding001()
	herd := body["holdings"].([]map[string]any)[0]
	body["holdings"] = []map[string]any{herd, herd}
	two := post(t, newLivestockQuotaRouter(&memoryLivestockQuotaRepo{}), testServiceKey, body)

	var r1, r2 dto.LivestockQuotaResponse
	require.NoError(t, json.Unmarshal(one.Body.Bytes(), &r1))
	require.NoError(t, json.Unmarshal(two.Body.Bytes(), &r2))
	assert.Equal(t, 2*r1.Head, r2.Head)
}

func TestLivestockQuotaRequiresTheServiceKey(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	r := newLivestockQuotaRouter(repo)

	assert.Equal(t, http.StatusUnauthorized, post(t, r, "", holding001()).Code)
	assert.Equal(t, http.StatusUnauthorized, post(t, r, "another-key", holding001()).Code)
	assert.Empty(t, repo.saved)
}

func TestLivestockQuotaRejectsInvalidRequests(t *testing.T) {
	cases := map[string]func(map[string]any){
		"no holdings":                  func(b map[string]any) { b["holdings"] = []map[string]any{} },
		"no national ID":               func(b map[string]any) { b["nationalId"] = "" },
		"unknown tenure":               func(b map[string]any) { b["tenure"] = "Comodato" },
		"negative head count":          func(b map[string]any) { b["holdings"].([]map[string]any)[0]["males1To2Years"] = -1 },
		"more completed than reported": func(b map[string]any) { b["completedCycles"] = 13 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &memoryLivestockQuotaRepo{}
			body := holding001()
			mutate(body)
			w := post(t, newLivestockQuotaRouter(repo), testServiceKey, body)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Empty(t, repo.saved)
		})
	}
}
