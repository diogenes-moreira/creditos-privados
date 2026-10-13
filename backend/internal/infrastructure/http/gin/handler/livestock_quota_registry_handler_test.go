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
