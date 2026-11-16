package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAssessments struct {
	byID map[uuid.UUID]*model.LivestockQuotaAssessment
}

func (f *fakeAssessments) Create(context.Context, *model.LivestockQuotaAssessment) error {
	panic("unexpected call")
}

func (f *fakeAssessments) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockQuotaAssessment, error) {
	return f.byID[id], nil
}

// fakeCreditLines enforces the unique indexes like Postgres does.
type fakeCreditLines struct {
	lines []*model.LivestockCreditLine
	docs  []*model.LivestockCreditLineDocument
	// hidden lines are stored but not found until the first Create fails,
	// to play a concurrent request that registered the line first.
	hidden []*model.LivestockCreditLine
}

func (f *fakeCreditLines) Create(_ context.Context, l *model.LivestockCreditLine, d *model.LivestockCreditLineDocument) error {
	if len(f.hidden) > 0 {
		f.lines = append(f.lines, f.hidden...)
		f.hidden = nil
		return errors.New("duplicate key value violates unique constraint")
	}
	for _, e := range f.lines {
		if e.PromissoryNoteNumber == l.PromissoryNoteNumber || e.AssessmentID == l.AssessmentID ||
			(e.Partner == l.Partner && e.PartnerReference == l.PartnerReference) {
			return errors.New("duplicate key value violates unique constraint")
		}
	}
	f.lines = append(f.lines, l)
	f.docs = append(f.docs, d)
	return nil
}

func (f *fakeCreditLines) find(match func(*model.LivestockCreditLine) bool) (*model.LivestockCreditLine, error) {
	for _, l := range f.lines {
		if match(l) {
			return l, nil
		}
	}
	return nil, nil
}

func (f *fakeCreditLines) FindByPromissoryNote(_ context.Context, n string) (*model.LivestockCreditLine, error) {
	return f.find(func(l *model.LivestockCreditLine) bool { return l.PromissoryNoteNumber == n })
}

func (f *fakeCreditLines) FindByPartnerReference(_ context.Context, p, r string) (*model.LivestockCreditLine, error) {
	return f.find(func(l *model.LivestockCreditLine) bool { return l.Partner == p && l.PartnerReference == r })
}

func (f *fakeCreditLines) FindByAssessment(_ context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	return f.find(func(l *model.LivestockCreditLine) bool { return l.AssessmentID == id })
}

var note = []byte("%PDF-1.7\nsigned\n%%EOF")

func creditFixture() (*service.LivestockCreditLineService, *fakeCreditLines, model.LivestockCreditActivation) {
	sum := sha256.Sum256(note)
	assessment := &model.LivestockQuotaAssessment{ID: uuid.New(), NationalID: "17788090", RequestedBy: "cuppogan",
		Quota: decimal.RequireFromString("1000000.00"), Currency: "COP"}
	lines := &fakeCreditLines{}
	s := service.NewLivestockCreditLineService(lines,
		&fakeAssessments{byID: map[uuid.UUID]*model.LivestockQuotaAssessment{assessment.ID: assessment}})
	return s, lines, model.LivestockCreditActivation{
		AssessmentID: assessment.ID, NationalID: "17788090", FullName: "GANADERO DE PRUEBA",
		Amount: decimal.RequireFromString("1000000"), Currency: "COP",
		PromissoryNoteNumber: "CPG-2026-000002", PartnerReference: "onboarding-1",
		SignedAt: time.Now(), ActivatedAt: time.Now(),
		DocumentSHA256: hex.EncodeToString(sum[:]), Document: note, DocumentContentType: "application/pdf",
	}
}

func code(t *testing.T, err error) string {
	t.Helper()
	r, ok := service.AsActivationRejection(err)
	require.True(t, ok, "expected a rejection, got %v", err)
	return r.Code
}

func TestRegisterActivationStoresTheLineWithItsDocument(t *testing.T) {
	s, lines, a := creditFixture()

	line, created, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	assert.True(t, created)
	require.Len(t, lines.lines, 1)
	require.Len(t, lines.docs, 1)
	assert.Equal(t, line.ID, lines.docs[0].CreditLineID)
	assert.Equal(t, note, lines.docs[0].Content)
	assert.Equal(t, "1000000.00", line.Amount.StringFixed(2))
}

func TestRegisterActivationIsIdempotent(t *testing.T) {
	s, lines, a := creditFixture()
	first, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)

	again, created, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Len(t, lines.lines, 1)
}

func TestRegisterActivationRejectsDifferentDataUnderTheSameNote(t *testing.T) {
	s, lines, a := creditFixture()
	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)

	other := a
	other.NationalID = "999"
	_, _, err = s.RegisterActivation(context.Background(), other, "cuppogan")
	assert.Equal(t, "promissory_note_already_registered", code(t, err))

	renumbered := a
	renumbered.PromissoryNoteNumber = "CPG-2026-000003"
	_, _, err = s.RegisterActivation(context.Background(), renumbered, "cuppogan")
	assert.Equal(t, "partner_reference_already_registered", code(t, err))
	assert.Len(t, lines.lines, 1)
}

func TestRegisterActivationChecksTheAssessment(t *testing.T) {
	s, lines, a := creditFixture()

	unknown := a
	unknown.AssessmentID = uuid.New()
	_, _, err := s.RegisterActivation(context.Background(), unknown, "cuppogan")
	assert.Equal(t, "unknown_assessment", code(t, err))

	otherID := a
	otherID.NationalID = "123456781"
	_, _, err = s.RegisterActivation(context.Background(), otherID, "cuppogan")
	assert.Equal(t, "national_id_mismatch", code(t, err))

	more := a
	more.Amount = decimal.RequireFromString("2000000")
	_, _, err = s.RegisterActivation(context.Background(), more, "cuppogan")
	assert.Equal(t, "amount_mismatch", code(t, err))

	_, _, err = s.RegisterActivation(context.Background(), a, "another-partner")
	assert.Equal(t, "assessment_of_another_partner", code(t, err))
	assert.Empty(t, lines.lines)
}

func TestRegisterActivationRejectsADocumentThatDoesNotMatchItsHash(t *testing.T) {
	s, lines, a := creditFixture()
	a.Document = []byte("%PDF-1.7 tampered")

	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	assert.Equal(t, "sha256_mismatch", code(t, err))
	assert.Empty(t, lines.lines)
}

func TestRegisterActivationLosingARaceAnswersWithTheWinner(t *testing.T) {
	s, lines, a := creditFixture()
	winner, _ := model.NewLivestockCreditLine(a, "cuppogan")
	lines.hidden = []*model.LivestockCreditLine{winner}

	line, created, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, winner.ID, line.ID)
}
