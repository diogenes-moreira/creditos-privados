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
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRUV answers like the registry: one record per predio and cycle.
type fakeRUV struct {
	ranchers map[string]*port.RUVRancher
	asked    []string
}

func (f *fakeRUV) FindByNationalID(_ context.Context, idType, id string) (*port.RUVRancher, error) {
	f.asked = append(f.asked, idType+"|"+id)
	return f.ranchers[id], nil
}

func newRegistryRouter(repo *memoryLivestockQuotaRepo, ruv port.RUVClient) *gin.Engine {
	return newRegistryRouterWith(repo, ruv, model.QuotaStrategyTiers)
}

func newRegistryRouterWith(repo *memoryLivestockQuotaRepo, ruv port.RUVClient, strategy model.QuotaStrategy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := service.NewLivestockQuotaService(repo, "COP").WithRegistry(ruv).WithStrategy(strategy)
	r.POST("/livestock-quotas/registry", middleware.RequireServiceKey("cuppogan", testServiceKey),
		handler.NewLivestockQuotaHandler(s).AssessFromRegistryForPartner)
	return r
}

func postRegistry(t *testing.T, r *gin.Engine, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/livestock-quotas/registry", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(middleware.ServiceKeyHeader, key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func caucaRancher() *port.RUVRancher {
	herd := model.Herd{FemalesUnder3Months: 3, Females3To8Months: 4, Females3To5Years: 14,
		MalesUnder3Months: 2, Males3To8Months: 3, Males2To3Years: 3, MalesOver3Years: 37}
	return &port.RUVRancher{NationalID: "17788090", Name: "GANADERO DE PRUEBA", Phone: "3000000000",
		Email: "ganadero@example.com", Holdings: []port.RUVHolding{
			// The same predio in an older cycle must not be summed twice.
			{Code: "100", Department: "CAUCA", Year: 2025, Cycle: 2, Herd: model.Herd{MalesOver3Years: 500}},
			{Code: "100", Department: "CAUCA", Year: 2026, Cycle: 1, Herd: herd},
			{Code: "200", Department: "CESAR", Year: 2026, Cycle: 1, Herd: model.Herd{Females3To5Years: 5}},
		}}
}

func TestPartnerRegistryQuotaReadsTheRUVAndSumsEachPredioOnce(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": caucaRancher()}}
	w := postRegistry(t, newRegistryRouter(repo, ruv), testServiceKey, map[string]any{
		"nationalId": "17788090", "idType": "CÉDULA DE CIUDADANÍA", "reference": "onboarding-42"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.PartnerRegistryQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"CÉDULA DE CIUDADANÍA|17788090"}, ruv.asked)
	assert.Equal(t, []string{"100", "200"}, resp.HoldingCodes)
	assert.Equal(t, 66+5, resp.Assessment.Head, "each predio at its latest cycle")
	assert.Equal(t, "onboarding-42", resp.Assessment.Reference)
	assert.NotEmpty(t, resp.Assumptions)
	assert.NotContains(t, w.Body.String(), "ganadero@example.com", "contact data is not returned")
	assert.NotContains(t, w.Body.String(), "3000000000", "contact data is not returned")
	assert.Equal(t, "GANADERO DE PRUEBA", resp.Rancher.Name)
	require.Len(t, resp.Rancher.Holdings, 3, "every predio and cycle, so the partner can pick its own")
	assert.Equal(t, dto.RegistryHoldingResponse{Code: "100", Department: "CAUCA", Year: 2026, Cycle: 1, Head: 66},
		resp.Rancher.Holdings[1])

	require.Len(t, repo.saved, 1)
	assert.Equal(t, "cuppogan", repo.saved[0].RequestedBy)
	assert.Equal(t, "onboarding-42", repo.saved[0].Reference)
}

func decodePartner(t *testing.T, w *httptest.ResponseRecorder) dto.PartnerRegistryQuotaResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.PartnerRegistryQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestPartnerRegistryQuotaGrantsTheTierMeasuredOnTheCurrentCycle(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": caucaRancher()}}
	resp := decodePartner(t, postRegistry(t, newRegistryRouter(repo, ruv), testServiceKey, map[string]any{
		"nationalId": "17788090", "idType": "CÉDULA DE CIUDADANÍA"}))

	// 2026-1: 66 + 5 heads, none unvaccinated. The 500 heads of 2025-2 are
	// out. Tier 3 needs an owned predio, which the RUV cannot show.
	a := resp.Assessment
	assert.Equal(t, "tiers", a.Strategy)
	assert.Equal(t, "1000000.00", a.Quota)
	assert.Equal(t, "COP", a.Currency)
	assert.Equal(t, "tier2", a.ActiveCeilingCode)
	assert.Equal(t, "Escalón 2", a.ActiveCeiling)
	assert.NotEqual(t, a.Quota, a.EngineQuota)
	require.NotNil(t, a.Tier)
	assert.Equal(t, "tier2", a.Tier.Code)
	assert.Equal(t, "2026-1", a.Tier.CurrentCycle)
	assert.True(t, a.Tier.CurrentCycleFound)
	assert.Equal(t, 71, a.Tier.VaccinatedHeads)
	assert.Equal(t, "1.0000", a.Tier.VaccinationRate)
	assert.Equal(t, []dto.QuotaTierReasonResponse{
		{Scope: "tier3", Requirement: "ownedHolding", Required: "true", Actual: "false"},
	}, a.Tier.Reasons)
	assert.Contains(t, resp.Assumptions, "tenure_from_ruv_propietario")
	assert.Equal(t, "GANADERO DE PRUEBA", resp.Rancher.Name, "the rancher stays in the response")

	require.Len(t, repo.saved, 1)
	saved := repo.saved[0]
	assert.Equal(t, "tier2", saved.Tier)
	assert.Equal(t, "1000000.00", saved.Quota.StringFixed(2))
	assert.Equal(t, a.EngineQuota, saved.EngineQuota.StringFixed(2))
}

func TestPartnerRegistryQuotaReachesTierThreeWhenPropietarioNamesTheRancher(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	owned := caucaRancher()
	// Only an old record of one predio says so: any record counts.
	owned.Holdings[0].Owner = "Sí"
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": owned}}
	resp := decodePartner(t, postRegistry(t, newRegistryRouter(repo, ruv), testServiceKey, map[string]any{"nationalId": "17788090"}))

	a := resp.Assessment
	assert.Equal(t, "2000000.00", a.Quota)
	require.NotNil(t, a.Tier)
	assert.Equal(t, "tier3", a.Tier.Code)
	assert.True(t, a.Tier.OwnedHolding)
	assert.Empty(t, a.Tier.Reasons)
	assert.Contains(t, resp.Assumptions, "tenure_from_ruv_propietario")
}

func TestPartnerRegistryQuotaAnswers200WithZeroWhenTheRancherReachesNoTier(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	stale := &port.RUVRancher{NationalID: "555", Holdings: []port.RUVHolding{
		{Code: "100", Department: "CAUCA", Year: 2025, Cycle: 2, Herd: model.Herd{MalesOver3Years: 500}},
	}}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"555": stale}}
	resp := decodePartner(t, postRegistry(t, newRegistryRouter(repo, ruv), testServiceKey, map[string]any{"nationalId": "555"}))

	a := resp.Assessment
	assert.Equal(t, "0.00", a.Quota)
	assert.Equal(t, "notEligible", a.ActiveCeilingCode)
	assert.Equal(t, "No elegible", a.ActiveCeiling)
	assert.True(t, decimal.RequireFromString(a.EngineQuota).IsPositive(), "the engine still prices the last known herd")
	require.NotNil(t, a.Tier)
	assert.False(t, a.Tier.CurrentCycleFound)
	assert.Equal(t, []dto.QuotaTierReasonResponse{
		{Scope: "entry", Requirement: "currentCycleRegistered", Required: "2026-1", Actual: "none"},
	}, a.Tier.Reasons)
	require.Len(t, repo.saved, 1)
	assert.Equal(t, "notEligible", repo.saved[0].Tier)
	assert.True(t, repo.saved[0].Quota.IsZero())
}

func TestPartnerRegistryQuotaUnderTheEngineStrategyGrantsTheEngineQuota(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": caucaRancher()}}
	resp := decodePartner(t, postRegistry(t, newRegistryRouterWith(repo, ruv, model.QuotaStrategyEngine), testServiceKey,
		map[string]any{"nationalId": "17788090"}))

	a := resp.Assessment
	assert.Equal(t, "engine", a.Strategy)
	assert.Equal(t, a.EngineQuota, a.Quota)
	assert.Nil(t, a.Tier)
	assert.NotContains(t, resp.Assumptions, "tenure_from_ruv_propietario")
	assert.Contains(t, []string{"ltv", "capacity", "inputs", "policyCap"}, a.ActiveCeilingCode)
	require.Len(t, repo.saved, 1)
	assert.Equal(t, "engine", repo.saved[0].Strategy)
	assert.Empty(t, repo.saved[0].Tier)
}

func TestPartnerRegistryQuotaAnswers404WhenTheRUVHasNoRecord(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	w := postRegistry(t, newRegistryRouter(repo, &fakeRUV{}), testServiceKey, map[string]any{"nationalId": "999"})

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, repo.saved)
}

func TestPartnerRegistryQuotaRequiresTheServiceKeyAndANationalID(t *testing.T) {
	r := newRegistryRouter(&memoryLivestockQuotaRepo{}, &fakeRUV{})

	assert.Equal(t, http.StatusUnauthorized, postRegistry(t, r, "", map[string]any{"nationalId": "1"}).Code)
	assert.Equal(t, http.StatusBadRequest, postRegistry(t, r, testServiceKey, map[string]any{}).Code)
}

func newAdvisorLookupRouter(repo *memoryLivestockQuotaRepo, ruv port.RUVClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := service.NewLivestockQuotaService(repo, "COP").WithRegistry(ruv)
	r.POST("/admin/livestock-quotas/registry", func(c *gin.Context) {
		c.Set("email", "asesor@example.com")
		c.Next()
	}, handler.NewLivestockQuotaHandler(s).AssessFromRegistry)
	return r
}

func TestAdvisorLookupTakesTheNationalIDInTheBodyAndRecordsWhoAsked(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": caucaRancher()}}
	r := newAdvisorLookupRouter(repo, ruv)

	raw, err := json.Marshal(map[string]any{"nationalId": " 17788090 ", "idType": "NIT"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/livestock-quotas/registry", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"NIT|17788090"}, ruv.asked)
	require.Len(t, repo.saved, 1)
	assert.Equal(t, "asesor@example.com", repo.saved[0].RequestedBy)

	// A NIT does not pass the tiers' entry filter.
	var resp dto.RegistryQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "0.00", resp.Assessment.Quota)
	require.NotNil(t, resp.Assessment.Tier)
	assert.Equal(t, "notEligible", resp.Assessment.Tier.Code)
	assert.Equal(t, []dto.QuotaTierReasonResponse{
		{Scope: "entry", Requirement: "cedula", Required: "true", Actual: "false"},
	}, resp.Assessment.Tier.Reasons)
}

func TestAdvisorLookupIsNotReachableByGet(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"17788090": caucaRancher()}}
	r := newAdvisorLookupRouter(repo, ruv)

	// The national ID must not ride in a URL, and a replayable read must not
	// query the registry or store an assessment.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/livestock-quotas/17788090", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, ruv.asked)
	assert.Empty(t, repo.saved)
}

func TestAdvisorLookupRejectsAMissingNationalID(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{}
	r := newAdvisorLookupRouter(repo, ruv)

	for _, body := range []string{`{}`, `{"nationalId":"   "}`} {
		req := httptest.NewRequest(http.MethodPost, "/admin/livestock-quotas/registry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
	assert.Empty(t, ruv.asked)
}
