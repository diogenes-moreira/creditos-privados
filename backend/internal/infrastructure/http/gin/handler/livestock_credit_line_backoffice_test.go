package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/auth"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleUsers is a user repository with one active user per role, keyed by the
// role's name as the identity UID.
type roleUsers struct {
	port.UserRepository
	users map[string]*model.User
}

func (r *roleUsers) FindByFirebaseUID(_ context.Context, uid string) (*model.User, error) {
	if u, ok := r.users[uid]; ok {
		return u, nil
	}
	return nil, errors.New("user not found")
}

// memoryAudit records audit entries.
type memoryAudit struct {
	port.AuditLogRepository
	logs []*model.AuditLog
}

func (m *memoryAudit) Create(_ context.Context, l *model.AuditLog) error {
	m.logs = append(m.logs, l)
	return nil
}

type backOfficeFixture struct {
	router     *gin.Engine
	auth       *auth.LocalAuthService
	users      *roleUsers
	lines      *memoryCreditLines
	audit      *memoryAudit
	assessment *model.LivestockQuotaAssessment
}

// newBackOfficeFixture mounts the routes the way the router does: under
// /admin, behind the bearer token and the advisor-or-admin role check.
func newBackOfficeFixture() backOfficeFixture {
	gin.SetMode(gin.TestMode)
	authService := auth.NewLocalAuthService("test-secret", 1)
	users := &roleUsers{users: map[string]*model.User{}}
	for _, role := range []model.UserRole{model.RoleAdmin, model.RoleAdvisor, model.RoleClient, model.RoleVendor} {
		users.users[string(role)] = &model.User{ID: uuid.New(), FirebaseUID: string(role), Role: role, IsActive: true}
	}
	assessment := &model.LivestockQuotaAssessment{ID: uuid.New(), NationalID: "17788090", RequestedBy: "cuppogan",
		Quota: decimal.RequireFromString("1000000.00"), Currency: "COP", Strategy: "tiers",
		Tier: "tier2", TierPolicyVersion: "v1", ActiveCeiling: "Escalón 2",
		EngineQuota: decimal.RequireFromString("2350000.00"), CreatedAt: time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)}
	quotas := &memoryLivestockQuotaRepo{saved: []*model.LivestockQuotaAssessment{assessment}}
	lines := &memoryCreditLines{}
	audit := &memoryAudit{}
	h := handler.NewLivestockCreditLineHandler(
		service.NewLivestockCreditLineService(lines, quotas).WithAudit(service.NewAuditService(audit)))

	r := gin.New()
	g := r.Group("/admin")
	g.Use(middleware.AuthMiddleware(authService), middleware.RequireRole(users, model.RoleAdmin, model.RoleAdvisor))
	h.RegisterBackOfficeRoutes(g)
	return backOfficeFixture{router: r, auth: authService, users: users, lines: lines, audit: audit, assessment: assessment}
}

// addLine stores an active line activated the given number of days after a
// fixed date, with signedPDF as its promissory note.
func (f backOfficeFixture) addLine(nationalID, note string, day int) *model.LivestockCreditLine {
	sum := sha256.Sum256(signedPDF)
	activated := time.Date(2026, 11, 1+day, 15, 0, 0, 0, time.UTC)
	line := &model.LivestockCreditLine{
		ID: uuid.New(), Status: model.LivestockCreditLineActive, AssessmentID: f.assessment.ID,
		NationalID: nationalID, IDType: "CÉDULA DE CIUDADANÍA", FullName: "GANADERO DE PRUEBA",
		MobilePhone: "+573000000000", Amount: decimal.RequireFromString("1000000.00"), Currency: "COP",
		Partner: "cuppogan", PartnerReference: "onboarding-" + note, PromissoryNoteNumber: note,
		SignedAt: activated.Add(-time.Minute), ActivatedAt: activated, CreatedAt: activated,
		SignatureProvider: "truora-zapsign", DocumentSHA256: hex.EncodeToString(sum[:]),
		DocumentSize: int64(len(signedPDF)),
	}
	f.lines.lines = append(f.lines.lines, line)
	f.lines.docs = append(f.lines.docs, &model.LivestockCreditLineDocument{CreditLineID: line.ID,
		ContentType: "application/pdf", SHA256: line.DocumentSHA256, Size: line.DocumentSize,
		Content: append([]byte(nil), signedPDF...)})
	return line
}

func (f backOfficeFixture) do(t *testing.T, role, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		token, err := f.auth.GenerateToken(role, role+"@example.com", role)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func decodePage(t *testing.T, w *httptest.ResponseRecorder) dto.LivestockCreditLinePage {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page dto.LivestockCreditLinePage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	return page
}

func TestCreditLineRoutesAreForAdvisorsAndAdmins(t *testing.T) {
	f := newBackOfficeFixture()
	line := f.addLine("17788090", "CPG-2026-000002", 1)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/admin/livestock-credit-lines"},
		{http.MethodPost, "/admin/livestock-credit-lines/search"},
		{http.MethodGet, "/admin/livestock-credit-lines/" + line.ID.String()},
		{http.MethodGet, "/admin/livestock-credit-lines/" + line.ID.String() + "/promissory-note"},
	}
	for _, rt := range routes {
		var body any
		if rt.method == http.MethodPost {
			body = map[string]any{}
		}
		assert.Equal(t, http.StatusUnauthorized, f.do(t, "", rt.method, rt.path, body).Code, rt.path)
		for _, role := range []string{"client", "vendor"} {
			assert.Equal(t, http.StatusForbidden, f.do(t, role, rt.method, rt.path, body).Code, role+" "+rt.path)
		}
		for _, role := range []string{"admin", "advisor"} {
			assert.Equal(t, http.StatusOK, f.do(t, role, rt.method, rt.path, body).Code, role+" "+rt.path)
		}
	}
}

func TestCreditLineListIsNewestFirstAndPaginated(t *testing.T) {
	f := newBackOfficeFixture()
	for day := 1; day <= 3; day++ {
		f.addLine(fmt.Sprintf("1000%d", day), fmt.Sprintf("CPG-2026-00000%d", day), day)
	}

	page := decodePage(t, f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines?limit=2", nil))
	assert.Equal(t, int64(3), page.Total)
	assert.Equal(t, 0, page.Offset)
	assert.Equal(t, 2, page.Limit)
	require.Len(t, page.Data, 2)
	assert.Equal(t, "CPG-2026-000003", page.Data[0].PromissoryNoteNumber)
	assert.Equal(t, "CPG-2026-000002", page.Data[1].PromissoryNoteNumber)

	page = decodePage(t, f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines?offset=2&limit=2", nil))
	require.Len(t, page.Data, 1)
	assert.Equal(t, "CPG-2026-000001", page.Data[0].PromissoryNoteNumber)

	// Defaults when no page is asked for.
	page = decodePage(t, f.do(t, "admin", http.MethodGet, "/admin/livestock-credit-lines", nil))
	assert.Equal(t, service.DefaultCreditLinePageSize, page.Limit)
	assert.Len(t, page.Data, 3)
}

func TestCreditLineListCarriesMetadataOnly(t *testing.T) {
	f := newBackOfficeFixture()
	line := f.addLine("17788090", "CPG-2026-000002", 1)

	w := f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines", nil)
	page := decodePage(t, w)
	require.Len(t, page.Data, 1)
	got := page.Data[0]
	assert.Equal(t, line.ID.String(), got.ID)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "17788090", got.NationalID)
	assert.Equal(t, "GANADERO DE PRUEBA", got.FullName)
	assert.Equal(t, "1000000.00", got.Amount)
	assert.Equal(t, "cuppogan", got.Partner)
	assert.Equal(t, line.DocumentSHA256, got.DocumentSHA256)
	assert.Equal(t, int64(len(signedPDF)), got.DocumentSize)
	assert.NotContains(t, w.Body.String(), "%PDF", "the promissory note is not listed")
}

func TestCreditLineSearchFiltersByNationalIDAndStatus(t *testing.T) {
	f := newBackOfficeFixture()
	f.addLine("17788090", "CPG-2026-000002", 1)
	f.addLine("1140867650", "CPG-2026-000003", 2)

	page := decodePage(t, f.do(t, "advisor", http.MethodPost, "/admin/livestock-credit-lines/search",
		map[string]any{"nationalId": " 17788090 ", "status": "active"}))
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Data, 1)
	assert.Equal(t, "CPG-2026-000002", page.Data[0].PromissoryNoteNumber)

	// Exact match only.
	page = decodePage(t, f.do(t, "advisor", http.MethodPost, "/admin/livestock-credit-lines/search",
		map[string]any{"nationalId": "1778809"}))
	assert.Equal(t, int64(0), page.Total)
	assert.NotNil(t, page.Data)

	w := f.do(t, "advisor", http.MethodPost, "/admin/livestock-credit-lines/search", map[string]any{"status": "closed"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "invalid_status", errorCode(t, w))
	w = f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines?status=closed", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreditLineDetailIncludesTheAssessment(t *testing.T) {
	f := newBackOfficeFixture()
	line := f.addLine("17788090", "CPG-2026-000002", 1)

	w := f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines/"+line.ID.String(), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got dto.LivestockCreditLineDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, line.ID.String(), got.ID)
	assert.Equal(t, "CPG-2026-000002", got.PromissoryNoteNumber)
	assert.Equal(t, "truora-zapsign", got.SignatureProvider)
	require.NotNil(t, got.Assessment)
	assert.Equal(t, f.assessment.ID.String(), got.Assessment.ID)
	assert.Equal(t, "1000000.00", got.Assessment.Quota)
	assert.Equal(t, "tiers", got.Assessment.Strategy)
	assert.Equal(t, "tier2", got.Assessment.Tier)
	assert.Equal(t, "Escalón 2", got.Assessment.TierLabel)
	assert.Equal(t, "2350000.00", got.Assessment.EngineQuota)
}

func TestCreditLineDetailAnswers404And400(t *testing.T) {
	f := newBackOfficeFixture()
	w := f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines/"+uuid.NewString(), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "credit_line_not_found", errorCode(t, w))

	w = f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines/"+uuid.NewString()+"/promissory-note", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPromissoryNoteStreamsThePDFAndAuditsTheAccess(t *testing.T) {
	f := newBackOfficeFixture()
	line := f.addLine("17788090", "CPG-2026-000002", 1)

	w := f.do(t, "advisor", http.MethodGet, "/admin/livestock-credit-lines/"+line.ID.String()+"/promissory-note", nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	assert.Equal(t, `inline; filename="CPG-2026-000002.pdf"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, fmt.Sprint(len(signedPDF)), w.Header().Get("Content-Length"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, signedPDF, w.Body.Bytes())

	require.Len(t, f.audit.logs, 1)
	entry := f.audit.logs[0]
	assert.Equal(t, "view_livestock_promissory_note", entry.Action)
	assert.Equal(t, line.ID.String(), entry.EntityID)
	require.NotNil(t, entry.UserID)
	assert.Equal(t, f.users.users["advisor"].ID, *entry.UserID)
}

func TestPromissoryNoteIsNotServedWhenItsHashNoLongerMatches(t *testing.T) {
	f := newBackOfficeFixture()
	line := f.addLine("17788090", "CPG-2026-000002", 1)
	f.lines.docs[0].Content[len(signedPDF)-1] = 'X'

	w := f.do(t, "admin", http.MethodGet, "/admin/livestock-credit-lines/"+line.ID.String()+"/promissory-note", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "promissory_note_corrupted", errorCode(t, w))
	assert.NotContains(t, w.Body.String(), "%PDF")
	require.Len(t, f.audit.logs, 1)
	assert.Equal(t, "promissory_note_integrity_failed", f.audit.logs[0].Action)
}
