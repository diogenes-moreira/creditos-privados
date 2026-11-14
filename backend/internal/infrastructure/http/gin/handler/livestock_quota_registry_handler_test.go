package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/gin-gonic/gin"
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
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := service.NewLivestockQuotaService(repo, "COP").WithRegistry(ruv)
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

// lookupDate fixes "today" for the registry history, so whether a holding
// counts as active does not drift as real time passes.
var lookupDate = time.Date(2026, time.October, 27, 0, 0, 0, 0, time.UTC)

func newAdvisorLookupRouter(repo *memoryLivestockQuotaRepo, ruv port.RUVClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := service.NewLivestockQuotaService(repo, "COP").WithRegistry(ruv).
		WithClock(func() time.Time { return lookupDate })
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

// fourCycleRancher vaccinated 12 head in each of the last four cycles, one of
// them leaving an animal out: enough for 500k, short of 1M's six cycles.
func fourCycleRancher() *port.RUVRancher {
	var hs []port.RUVHolding
	for i, yc := range [][2]int{{2024, 2}, {2025, 1}, {2025, 2}, {2026, 1}} {
		nv := 0
		if i == 0 {
			nv = 1
		}
		hs = append(hs, port.RUVHolding{Code: "300", Department: "META", Year: yc[0], Cycle: yc[1],
			Herd: model.Herd{Females3To5Years: 12, NotVaccinated: nv}})
	}
	// As if the registry returned the whole history, which getRuvScoring
	// does not: this exercises the layers once a history source exists.
	return &port.RUVRancher{NationalID: "12345678", Name: "GANADERO CUATRO CICLOS", Holdings: hs, HistoryComplete: true}
}

func postAdvisorLookup(t *testing.T, r *gin.Engine, id string) dto.RegistryQuotaResponse {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"nationalId": id})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/livestock-quotas/registry", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.RegistryQuotaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestAdvisorLookupWithOnlyTheCurrentCycleOffersAProvisionalLayer(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	// What getRuvScoring actually returns: the current cycle, no history.
	current := &port.RUVRancher{NationalID: "87654321", Name: "GANADERO CICLO ACTUAL", Holdings: []port.RUVHolding{
		{Code: "400", Department: "CESAR", Year: 2026, Cycle: 1, Herd: model.Herd{Females3To5Years: 30, MalesOver3Years: 5}},
	}}
	r := newAdvisorLookupRouter(repo, &fakeRUV{ranchers: map[string]*port.RUVRancher{"87654321": current}})

	tier := postAdvisorLookup(t, r, "87654321").CreditTier

	assert.Empty(t, tier.Code, "no layer is granted on cycles nobody has seen")
	assert.Equal(t, "0.00", tier.Amount)
	assert.Equal(t, "1M", tier.ProvisionalCode, "35 head, all vaccinated")
	assert.Equal(t, "1000000.00", tier.ProvisionalAmount)
	assert.False(t, tier.Vaccination.HistoryKnown)

	require.Len(t, repo.saved, 1)
	assert.Equal(t, "1M", repo.saved[0].ProvisionalTier)
	assert.Equal(t, "1000000", repo.saved[0].ProvisionalTierAmount.String())
}

func TestAdvisorLookupReturnsAndStoresTheCreditTier(t *testing.T) {
	repo := &memoryLivestockQuotaRepo{}
	ruv := &fakeRUV{ranchers: map[string]*port.RUVRancher{"12345678": fourCycleRancher()}}
	r := newAdvisorLookupRouter(repo, ruv)

	resp := postAdvisorLookup(t, r, "12345678")

	tier := resp.CreditTier
	assert.Equal(t, "500k", tier.Code)
	assert.Equal(t, "500000.00", tier.Amount)
	assert.True(t, tier.Eligible)
	assert.ElementsMatch(t, []string{model.CriterionCampaignBuyer, model.CriterionCleanBureau}, tier.Pending,
		"neither is in the registry, so both stay to be verified")
	assert.Equal(t, 12, tier.Vaccination.VaccinatedHeads)
	assert.Equal(t, 4, tier.Vaccination.ReportedCycles)
	assert.Equal(t, 3, tier.Vaccination.CompleteCycles)
	require.Len(t, tier.Tiers, 3)

	require.Len(t, repo.saved, 1)
	assert.Equal(t, "500k", repo.saved[0].CreditTier, "every lookup record keeps the amount it showed")
	assert.Equal(t, "500000", repo.saved[0].CreditTierAmount.String())
}
