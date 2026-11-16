package model_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var signedNote = []byte("%PDF-1.7\nsigned promissory note\n%%EOF")

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func activation() model.LivestockCreditActivation {
	return model.LivestockCreditActivation{
		AssessmentID:         uuid.New(),
		NationalID:           " 17788090 ",
		IDType:               "CÉDULA DE CIUDADANÍA",
		FullName:             "GANADERO DE PRUEBA",
		MobilePhone:          "+573000000000",
		Amount:               decimal.RequireFromString("1000000"),
		Currency:             "cop",
		PromissoryNoteNumber: "CPG-2026-000002",
		SignedAt:             time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC),
		ActivatedAt:          time.Date(2026, 11, 16, 15, 1, 0, 0, time.UTC),
		DocumentSHA256:       sum(signedNote),
		PartnerReference:     "onboarding-1",
		Document:             signedNote,
		DocumentContentType:  "application/pdf",
	}
}

func rejection(t *testing.T, err error) *model.ActivationRejection {
	t.Helper()
	var r *model.ActivationRejection
	require.ErrorAs(t, err, &r)
	return r
}

func TestActivationValidateNormalizes(t *testing.T) {
	a := activation()
	require.NoError(t, a.Validate())
	assert.Equal(t, "17788090", a.NationalID)
	assert.Equal(t, "COP", a.Currency)
}

func TestActivationValidateRejectsBadDocuments(t *testing.T) {
	cases := map[string]struct {
		mutate func(*model.LivestockCreditActivation)
		kind   model.ActivationRejectionKind
		code   string
	}{
		"missing assessment": {func(a *model.LivestockCreditActivation) { a.AssessmentID = uuid.Nil }, model.ActivationInvalid, "invalid_request"},
		"zero amount":        {func(a *model.LivestockCreditActivation) { a.Amount = decimal.Zero }, model.ActivationInvalid, "invalid_request"},
		"no document":        {func(a *model.LivestockCreditActivation) { a.Document = nil }, model.ActivationInvalid, "invalid_request"},
		"bad hash format":    {func(a *model.LivestockCreditActivation) { a.DocumentSHA256 = "abc" }, model.ActivationInvalid, "invalid_request"},
		"not a pdf content type": {func(a *model.LivestockCreditActivation) { a.DocumentContentType = "image/png" },
			model.ActivationUnprocessable, "not_a_pdf"},
		"not pdf bytes": {func(a *model.LivestockCreditActivation) {
			a.Document = []byte("<html>")
			a.DocumentSHA256 = sum(a.Document)
		}, model.ActivationUnprocessable, "not_a_pdf"},
		"hash mismatch": {func(a *model.LivestockCreditActivation) { a.DocumentSHA256 = sum([]byte("other")) },
			model.ActivationUnprocessable, "sha256_mismatch"},
		"too large": {func(a *model.LivestockCreditActivation) {
			a.Document = append([]byte("%PDF-"), bytes.Repeat([]byte("x"), model.MaxPromissoryNoteSize)...)
		}, model.ActivationTooLarge, "document_too_large"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := activation()
			tc.mutate(&a)
			r := rejection(t, a.Validate())
			assert.Equal(t, tc.kind, r.Kind)
			assert.Equal(t, tc.code, r.Code)
		})
	}
}

func TestActivationContentTypeParametersAreAccepted(t *testing.T) {
	a := activation()
	a.DocumentContentType = "application/pdf; name=CPG-2026-000002.pdf"
	assert.NoError(t, a.Validate())
}

func assessmentFor(a model.LivestockCreditActivation) *model.LivestockQuotaAssessment {
	return &model.LivestockQuotaAssessment{ID: a.AssessmentID, NationalID: "17788090", RequestedBy: "cuppogan",
		Quota: decimal.RequireFromString("1000000.00"), Currency: "COP"}
}

func TestActivationCheckAgainstTheAssessment(t *testing.T) {
	a := activation()
	require.NoError(t, a.Validate())
	require.NoError(t, a.CheckAgainst(assessmentFor(a), "cuppogan"))

	other := assessmentFor(a)
	other.NationalID = "123"
	assert.Equal(t, "national_id_mismatch", rejection(t, a.CheckAgainst(other, "cuppogan")).Code)

	assert.Equal(t, "assessment_of_another_partner", rejection(t, a.CheckAgainst(assessmentFor(a), "other")).Code)

	lower := assessmentFor(a)
	lower.Quota = decimal.RequireFromString("500000")
	r := rejection(t, a.CheckAgainst(lower, "cuppogan"))
	assert.Equal(t, "amount_mismatch", r.Code)
	assert.Equal(t, model.ActivationUnprocessable, r.Kind)

	zero := assessmentFor(a)
	zero.Quota = decimal.Zero
	assert.Equal(t, "assessment_without_quota", rejection(t, a.CheckAgainst(zero, "cuppogan")).Code)

	usd := assessmentFor(a)
	usd.Currency = "USD"
	assert.Equal(t, "currency_mismatch", rejection(t, a.CheckAgainst(usd, "cuppogan")).Code)
}

func TestNewLivestockCreditLineKeepsTheDocumentApart(t *testing.T) {
	a := activation()
	require.NoError(t, a.Validate())
	line, doc := model.NewLivestockCreditLine(a, "cuppogan")

	assert.Equal(t, model.LivestockCreditLineActive, line.Status)
	assert.Equal(t, "cuppogan", line.Partner)
	assert.Equal(t, int64(len(signedNote)), line.DocumentSize)
	assert.Equal(t, line.ID, doc.CreditLineID)
	assert.Equal(t, signedNote, doc.Content)
	assert.Equal(t, line.DocumentSHA256, doc.SHA256)
	assert.True(t, line.Replays(a, "cuppogan"))
}

func TestCreditLineReplayAndConflicts(t *testing.T) {
	a := activation()
	require.NoError(t, a.Validate())
	line, _ := model.NewLivestockCreditLine(a, "cuppogan")

	more := a
	more.Amount = decimal.RequireFromString("2000000")
	assert.False(t, line.Replays(more, "cuppogan"))
	r := rejection(t, line.ConflictWith(more, "cuppogan"))
	assert.Equal(t, model.ActivationConflict, r.Kind)
	assert.Equal(t, "promissory_note_already_registered", r.Code)

	renumbered := a
	renumbered.PromissoryNoteNumber = "CPG-2026-000003"
	assert.Equal(t, "partner_reference_already_registered", rejection(t, line.ConflictWith(renumbered, "cuppogan")).Code)

	reused := a
	reused.PromissoryNoteNumber = "CPG-2026-000003"
	reused.PartnerReference = "onboarding-2"
	assert.Equal(t, "assessment_already_activated", rejection(t, line.ConflictWith(reused, "cuppogan")).Code)
}
