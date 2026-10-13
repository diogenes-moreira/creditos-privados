package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryPolicyRepo counts writes so a test can prove a read made none.
type memoryPolicyRepo struct {
	policies []*model.LivestockQuotaPolicy
	writes   int
}

func (m *memoryPolicyRepo) Create(_ context.Context, p *model.LivestockQuotaPolicy) error {
	m.writes++
	m.policies = append(m.policies, p)
	return nil
}

func (m *memoryPolicyRepo) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockQuotaPolicy, error) {
	for _, p := range m.policies {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *memoryPolicyRepo) FindActive(_ context.Context) (*model.LivestockQuotaPolicy, error) {
	for _, p := range m.policies {
		if p.IsActive {
			return p, nil
		}
	}
	return nil, errors.New("none active")
}

func (m *memoryPolicyRepo) FindAll(_ context.Context, _, _ int) ([]model.LivestockQuotaPolicy, int64, error) {
	return nil, 0, nil
}

func (m *memoryPolicyRepo) Update(_ context.Context, _ *model.LivestockQuotaPolicy) error {
	m.writes++
	return nil
}

func (m *memoryPolicyRepo) ReplaceFactors(_ context.Context, _ uuid.UUID, _ []model.LivestockUGGFactor) error {
	m.writes++
	return nil
}

func (m *memoryPolicyRepo) SetActive(_ context.Context, _ uuid.UUID) error {
	m.writes++
	return nil
}

func getActivePolicy(repo *memoryPolicyRepo) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// No audit service: a read must never get as far as recording anything.
	h := handler.NewLivestockQuotaPolicyHandler(service.NewLivestockQuotaPolicyService(repo, nil))
	r.GET("/admin/livestock-quota-policies/active", h.GetActiveLivestockQuotaPolicy)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/livestock-quota-policies/active", nil))
	return w
}

func TestReadingTheActivePolicyNeverCreatesOne(t *testing.T) {
	repo := &memoryPolicyRepo{}

	w := getActivePolicy(repo)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Zero(t, repo.writes, "a GET must not seed or activate a policy")
}

func TestReadingTheActivePolicyReturnsTheStoredOne(t *testing.T) {
	p, err := model.DefaultLivestockQuotaPolicy(nil)
	require.NoError(t, err)
	p.Activate()
	repo := &memoryPolicyRepo{policies: []*model.LivestockQuotaPolicy{p}}

	w := getActivePolicy(repo)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), p.ID.String())
	assert.Zero(t, repo.writes)
}
