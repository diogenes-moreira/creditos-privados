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
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
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
	listed []port.LivestockCreditLineFilter
	// enrollments are the ones stored with Create or LinkClient.
	enrollments []*model.LivestockClientEnrollment
}

func (f *fakeCreditLines) LinkClient(_ context.Context, l *model.LivestockCreditLine, e *model.LivestockClientEnrollment) error {
	f.enroll(l, e)
	return nil
}

// enroll plays the repository: keeps the enrollment and points the line at it.
func (f *fakeCreditLines) enroll(l *model.LivestockCreditLine, e *model.LivestockClientEnrollment) {
	if e == nil {
		return
	}
	f.enrollments = append(f.enrollments, e)
	l.ClientID, l.CreditLineID = &e.Client.ID, &e.CreditLine.ID
}

func (f *fakeCreditLines) Create(_ context.Context, l *model.LivestockCreditLine, d *model.LivestockCreditLineDocument, enr *model.LivestockClientEnrollment) error {
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
	f.enroll(l, enr)
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

func (f *fakeCreditLines) FindByID(_ context.Context, id uuid.UUID) (*model.LivestockCreditLine, error) {
	return f.find(func(l *model.LivestockCreditLine) bool { return l.ID == id })
}

// List records the filter it was asked for and returns every line.
func (f *fakeCreditLines) List(_ context.Context, filter port.LivestockCreditLineFilter) ([]model.LivestockCreditLine, int64, error) {
	f.listed = append(f.listed, filter)
	out := make([]model.LivestockCreditLine, 0, len(f.lines))
	for _, l := range f.lines {
		out = append(out, *l)
	}
	return out, int64(len(out)), nil
}

func (f *fakeCreditLines) FindDocument(_ context.Context, id uuid.UUID) (*model.LivestockCreditLineDocument, error) {
	for _, d := range f.docs {
		if d.CreditLineID == id {
			return d, nil
		}
	}
	return nil, nil
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

func TestListNormalizesThePageAndRejectsUnknownStatuses(t *testing.T) {
	s, lines, _ := creditFixture()

	_, _, f, err := s.List(context.Background(), port.LivestockCreditLineFilter{Offset: -3})
	require.NoError(t, err)
	assert.Equal(t, 0, f.Offset)
	assert.Equal(t, service.DefaultCreditLinePageSize, f.Limit)

	_, _, f, err = s.List(context.Background(), port.LivestockCreditLineFilter{Limit: 5000, NationalID: "17788090", Status: model.LivestockCreditLineActive})
	require.NoError(t, err)
	assert.Equal(t, service.MaxCreditLinePageSize, f.Limit)
	assert.Equal(t, "17788090", lines.listed[1].NationalID)

	_, _, _, err = s.List(context.Background(), port.LivestockCreditLineFilter{Status: "closed"})
	assert.ErrorIs(t, err, service.ErrInvalidCreditLineStatus)
	assert.Len(t, lines.listed, 2)
}

func TestGetReturnsTheLineWithItsAssessment(t *testing.T) {
	s, _, a := creditFixture()
	line, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)

	got, assessment, err := s.Get(context.Background(), line.ID)
	require.NoError(t, err)
	assert.Equal(t, line.ID, got.ID)
	require.NotNil(t, assessment)
	assert.Equal(t, a.AssessmentID, assessment.ID)

	_, _, err = s.Get(context.Background(), uuid.New())
	assert.ErrorIs(t, err, service.ErrCreditLineNotFound)
}

func TestPromissoryNoteChecksTheHashAndAuditsEveryAccess(t *testing.T) {
	s, lines, a := creditFixture()
	audit := &fakeAudit{}
	s.WithAudit(service.NewAuditService(audit))
	line, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	viewer := uuid.New()

	_, doc, err := s.PromissoryNote(context.Background(), line.ID, &viewer)
	require.NoError(t, err)
	assert.Equal(t, note, doc.Content)
	last := audit.logs[len(audit.logs)-1]
	assert.Equal(t, "view_livestock_promissory_note", last.Action)
	assert.Equal(t, &viewer, last.UserID)
	assert.Equal(t, line.ID.String(), last.EntityID)

	// A byte changed in storage: the note is not served, and the failure is recorded.
	tampered := append([]byte(nil), note...)
	tampered[len(tampered)-1] = 'X'
	lines.docs[0].Content = tampered
	_, _, err = s.PromissoryNote(context.Background(), line.ID, &viewer)
	assert.ErrorIs(t, err, service.ErrPromissoryNoteCorrupted)
	assert.Equal(t, "promissory_note_integrity_failed", audit.logs[len(audit.logs)-1].Action)

	_, _, err = s.PromissoryNote(context.Background(), uuid.New(), &viewer)
	assert.ErrorIs(t, err, service.ErrCreditLineNotFound)
}
