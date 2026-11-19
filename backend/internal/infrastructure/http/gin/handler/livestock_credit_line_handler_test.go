package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sort"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/handler"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/http/gin/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryCreditLines struct {
	lines []*model.LivestockCreditLine
	docs  []*model.LivestockCreditLineDocument
}

func (m *memoryCreditLines) Create(_ context.Context, l *model.LivestockCreditLine, d *model.LivestockCreditLineDocument, e *model.LivestockClientEnrollment) error {
	m.LinkClient(context.Background(), l, e)
	m.lines = append(m.lines, l)
	m.docs = append(m.docs, d)
	return nil
}

func (m *memoryCreditLines) LinkClient(_ context.Context, l *model.LivestockCreditLine, e *model.LivestockClientEnrollment) error {
	if e != nil {
		l.ClientID, l.CreditLineID = &e.Client.ID, &e.CreditLine.ID
	}
	return nil
}

func (m *memoryCreditLines) CreateLoan(context.Context, *model.LivestockCreditLine, *model.CreditLine, *model.Loan) error {
	return nil
}

func (m *memoryCreditLines) find(match func(*model.LivestockCreditLine) bool) (*model.LivestockCreditLine, error) {
	for _, l := range m.lines {
		if match(l) {
			return l, nil
		}
	}
	return nil, nil
}

func (m *memoryCreditLines) FindByPromissoryNote(_ context.Context, n string) (*model.LivestockCreditLine, error) {
	return m.find(func(l *model.LivestockCreditLine) bool { return l.PromissoryNoteNumber == n })
}

func (m *memoryCreditLines) FindByPartnerReference(_ context.Context, p, r string) (*model.LivestockCreditLine, error) {
	return m.find(func(l *model.LivestockCreditLine) bool { return l.Partner == p && l.PartnerReference == r })
}

func (m *memoryCreditLines) FindByAssessment(_ context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	return m.find(func(l *model.LivestockCreditLine) bool { return l.AssessmentID == id })
}

func (m *memoryCreditLines) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	return m.find(func(l *model.LivestockCreditLine) bool { return l.ID == id })
}

// List filters like the Postgres repository: exact national ID and status,
// most recently activated first, then the page.
func (m *memoryCreditLines) List(_ context.Context, f port.LivestockCreditLineFilter) ([]model.LivestockCreditLine, int64, error) {
	var matched []model.LivestockCreditLine
	for _, l := range m.lines {
		if (f.NationalID == "" || l.NationalID == f.NationalID) && (f.Status == "" || l.Status == f.Status) {
			matched = append(matched, *l)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].ActivatedAt.After(matched[j].ActivatedAt) })
	total := int64(len(matched))
	if f.Offset >= len(matched) {
		return []model.LivestockCreditLine{}, total, nil
	}
	matched = matched[f.Offset:]
	if f.Limit < len(matched) {
		matched = matched[:f.Limit]
	}
	return matched, total, nil
}

func (m *memoryCreditLines) FindDocument(_ context.Context, id uuid.UUID) (*model.LivestockCreditLineDocument, error) {
	for _, d := range m.docs {
		if d.CreditLineID == id {
			return d, nil
		}
	}
	return nil, nil
}

var signedPDF = []byte("%PDF-1.7\n1 0 obj << >> endobj\n%%EOF")

type activationFixture struct {
	router     *gin.Engine
	lines      *memoryCreditLines
	assessment *model.LivestockQuotaAssessment
}

func newActivationFixture() activationFixture {
	gin.SetMode(gin.TestMode)
	assessment := &model.LivestockQuotaAssessment{ID: uuid.New(), NationalID: "17788090", RequestedBy: "cuppogan",
		Reference: "onboarding-7", Quota: decimal.RequireFromString("1000000.00"), Currency: "COP"}
	quotas := &memoryLivestockQuotaRepo{saved: []*model.LivestockQuotaAssessment{assessment}}
	lines := &memoryCreditLines{}
	r := gin.New()
	r.POST("/livestock-quotas/activations", middleware.RequireServiceKey("cuppogan", testServiceKey),
		handler.NewLivestockCreditLineHandler(service.NewLivestockCreditLineService(lines, quotas)).RegisterActivation)
	return activationFixture{router: r, lines: lines, assessment: assessment}
}

func (f activationFixture) metadata(document []byte) map[string]any {
	sum := sha256.Sum256(document)
	return map[string]any{
		"assessmentId": f.assessment.ID.String(), "nationalId": "17788090", "idType": "CÉDULA DE CIUDADANÍA",
		"fullName": "GANADERO DE PRUEBA", "mobilePhone": "+573000000000",
		"amount": "1000000.00", "currency": "COP", "promissoryNoteNumber": "CPG-2026-000002",
		"signedAt": "2026-11-16T15:00:00Z", "activatedAt": "2026-11-16T15:01:00Z",
		"documentSha256": hex.EncodeToString(sum[:]), "partnerReference": "onboarding-7",
		"archiveUri": "gs://pagares/CPG-2026-000002.pdf", "signatureProvider": "truora-zapsign",
		"signatureValidationId": "VLD-1",
	}
}

type activationPart struct {
	metadata        any
	metadataAsFile  bool
	document        []byte
	documentType    string
	withoutDocument bool
}

func postActivation(t *testing.T, r *gin.Engine, key string, p activationPart) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	raw, err := json.Marshal(p.metadata)
	require.NoError(t, err)
	if p.metadataAsFile {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="metadata"; filename="metadata.json"`)
		h.Set("Content-Type", "application/json")
		part, err := w.CreatePart(h)
		require.NoError(t, err)
		_, _ = part.Write(raw)
	} else {
		require.NoError(t, w.WriteField("metadata", string(raw)))
	}
	if !p.withoutDocument {
		contentType := p.documentType
		if contentType == "" {
			contentType = "application/pdf"
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="promissoryNote"; filename="CPG-2026-000002.pdf"`)
		h.Set("Content-Type", contentType)
		part, err := w.CreatePart(h)
		require.NoError(t, err)
		_, _ = part.Write(p.document)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/livestock-quotas/activations", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if key != "" {
		req.Header.Set(middleware.ServiceKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp dto.CodedErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return resp.Code
}

func TestActivationRegistersTheCreditLineWithThePDF(t *testing.T) {
	f := newActivationFixture()
	w := postActivation(t, f.router, testServiceKey, activationPart{metadata: f.metadata(signedPDF), document: signedPDF})

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp dto.LivestockCreditLineResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "active", resp.Status)
	assert.Equal(t, f.assessment.ID.String(), resp.AssessmentID)
	assert.Equal(t, "1000000.00", resp.Amount)
	assert.Equal(t, "CPG-2026-000002", resp.PromissoryNoteNumber)
	assert.Equal(t, int64(len(signedPDF)), resp.DocumentSize)
	assert.NotContains(t, w.Body.String(), "GANADERO", "personal data is not echoed")
	assert.NotContains(t, w.Body.String(), "+573000000000", "personal data is not echoed")

	require.Len(t, f.lines.lines, 1)
	line := f.lines.lines[0]
	assert.Equal(t, "cuppogan", line.Partner)
	assert.Equal(t, "onboarding-7", line.PartnerReference)
	assert.Equal(t, "GANADERO DE PRUEBA", line.FullName)
	assert.Equal(t, "VLD-1", line.SignatureValidationID)
	assert.Equal(t, "gs://pagares/CPG-2026-000002.pdf", line.PartnerArchiveURI)
	assert.Equal(t, signedPDF, f.lines.docs[0].Content)
	assert.Equal(t, "application/pdf", f.lines.docs[0].ContentType)
}

func TestActivationReplayReturnsTheSameLine(t *testing.T) {
	f := newActivationFixture()
	part := activationPart{metadata: f.metadata(signedPDF), document: signedPDF}
	first := postActivation(t, f.router, testServiceKey, part)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	again := postActivation(t, f.router, testServiceKey, part)

	require.Equal(t, http.StatusOK, again.Code, again.Body.String())
	var a, b dto.LivestockCreditLineResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(again.Body.Bytes(), &b))
	assert.Equal(t, a.ID, b.ID)
	assert.Len(t, f.lines.lines, 1)
}

func TestActivationWithDifferentDataForARegisteredNoteIsAConflict(t *testing.T) {
	f := newActivationFixture()
	require.Equal(t, http.StatusCreated,
		postActivation(t, f.router, testServiceKey, activationPart{metadata: f.metadata(signedPDF), document: signedPDF}).Code)

	other := []byte("%PDF-1.7 another signed note")
	w := postActivation(t, f.router, testServiceKey, activationPart{metadata: f.metadata(other), document: other})

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "promissory_note_already_registered", errorCode(t, w))
	assert.Len(t, f.lines.lines, 1)
}

func TestActivationAcceptsTheMetadataAsAJSONPart(t *testing.T) {
	f := newActivationFixture()
	w := postActivation(t, f.router, testServiceKey,
		activationPart{metadata: f.metadata(signedPDF), metadataAsFile: true, document: signedPDF})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestActivationRejections(t *testing.T) {
	notPDF := []byte("<html>not a pdf</html>")
	cases := map[string]struct {
		part   func(f activationFixture) activationPart
		status int
		code   string
	}{
		"hash mismatch": {func(f activationFixture) activationPart {
			return activationPart{metadata: f.metadata([]byte("%PDF-1.7 other")), document: signedPDF}
		}, http.StatusUnprocessableEntity, "sha256_mismatch"},
		"not a pdf": {func(f activationFixture) activationPart {
			return activationPart{metadata: f.metadata(notPDF), document: notPDF}
		}, http.StatusUnprocessableEntity, "not_a_pdf"},
		"not declared as a pdf": {func(f activationFixture) activationPart {
			return activationPart{metadata: f.metadata(signedPDF), document: signedPDF, documentType: "application/octet-stream"}
		}, http.StatusUnprocessableEntity, "not_a_pdf"},
		"unknown assessment": {func(f activationFixture) activationPart {
			m := f.metadata(signedPDF)
			m["assessmentId"] = uuid.NewString()
			return activationPart{metadata: m, document: signedPDF}
		}, http.StatusUnprocessableEntity, "unknown_assessment"},
		"assessment of another national id": {func(f activationFixture) activationPart {
			m := f.metadata(signedPDF)
			m["nationalId"] = "123456781"
			return activationPart{metadata: m, document: signedPDF}
		}, http.StatusUnprocessableEntity, "national_id_mismatch"},
		"amount other than the quota": {func(f activationFixture) activationPart {
			m := f.metadata(signedPDF)
			m["amount"] = "2000000.00"
			return activationPart{metadata: m, document: signedPDF}
		}, http.StatusUnprocessableEntity, "amount_mismatch"},
		"assessment id not a uuid": {func(f activationFixture) activationPart {
			m := f.metadata(signedPDF)
			m["assessmentId"] = "LQA-1"
			return activationPart{metadata: m, document: signedPDF}
		}, http.StatusBadRequest, "invalid_request"},
		"metadata not json": {func(f activationFixture) activationPart {
			return activationPart{metadata: "{", document: signedPDF}
		}, http.StatusBadRequest, "invalid_request"},
		"no document": {func(f activationFixture) activationPart {
			return activationPart{metadata: f.metadata(signedPDF), withoutDocument: true}
		}, http.StatusBadRequest, "invalid_request"},
		"document too large": {func(f activationFixture) activationPart {
			big := append(append([]byte{}, signedPDF...), bytes.Repeat([]byte("x"), model.MaxPromissoryNoteSize)...)
			return activationPart{metadata: f.metadata(big), document: big}
		}, http.StatusRequestEntityTooLarge, "document_too_large"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newActivationFixture()
			w := postActivation(t, f.router, testServiceKey, tc.part(f))
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, tc.code, errorCode(t, w))
			assert.Empty(t, f.lines.lines)
		})
	}
}

func TestActivationRequiresTheServiceKey(t *testing.T) {
	f := newActivationFixture()
	part := activationPart{metadata: f.metadata(signedPDF), document: signedPDF}

	assert.Equal(t, http.StatusUnauthorized, postActivation(t, f.router, "", part).Code)
	assert.Equal(t, http.StatusUnauthorized, postActivation(t, f.router, "wrong-key", part).Code)
	assert.Empty(t, f.lines.lines)
}
