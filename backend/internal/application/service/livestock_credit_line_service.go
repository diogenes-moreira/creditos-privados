package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
)

// Credit line listing page sizes.
const (
	DefaultCreditLinePageSize = 20
	MaxCreditLinePageSize     = 100
)

var (
	// ErrCreditLineNotFound is a credit line, or its document, that does not exist.
	ErrCreditLineNotFound = errors.New("livestock credit line not found")
	// ErrInvalidCreditLineStatus is a status filter no line can have.
	ErrInvalidCreditLineStatus = errors.New("unknown credit line status")
	// ErrPromissoryNoteCorrupted is a stored promissory note whose bytes no
	// longer hash to the SHA-256 registered with the line. It is never served.
	ErrPromissoryNoteCorrupted = errors.New("the stored promissory note does not match its registered SHA-256")
)

// LivestockCreditLineService registers the credit lines partners activate on
// a livestock quota. For now it only records them, with the signed promissory
// note; Prestia will manage the credit from these records later.
type LivestockCreditLineService struct {
	lines       port.LivestockCreditLineRepository
	assessments port.LivestockQuotaRepository
	audit       *AuditService
}

func NewLivestockCreditLineService(lines port.LivestockCreditLineRepository, assessments port.LivestockQuotaRepository) *LivestockCreditLineService {
	return &LivestockCreditLineService{lines: lines, assessments: assessments}
}

// WithAudit records every registered line in the audit log.
func (s *LivestockCreditLineService) WithAudit(audit *AuditService) *LivestockCreditLineService {
	s.audit = audit
	return s
}

// RegisterActivation opens the credit line an activation describes. It is
// idempotent: a replay of an activation already registered (same promissory
// note, partner reference, assessment, rancher, amount and document) returns
// that line with created false. Rejections are *model.ActivationRejection.
func (s *LivestockCreditLineService) RegisterActivation(ctx context.Context, a model.LivestockCreditActivation, partner string) (*model.LivestockCreditLine, bool, error) {
	if err := a.Validate(); err != nil {
		return nil, false, err
	}
	if line, err := s.existing(ctx, a, partner); line != nil || err != nil {
		return line, false, err
	}

	assessment, err := s.assessments.FindByID(ctx, a.AssessmentID)
	if err != nil {
		return nil, false, fmt.Errorf("reading the assessment: %w", err)
	}
	if assessment == nil {
		return nil, false, &model.ActivationRejection{Kind: model.ActivationUnprocessable,
			Code: "unknown_assessment", Message: "no quota assessment with that id"}
	}
	if err := a.CheckAgainst(assessment, partner); err != nil {
		return nil, false, err
	}

	line, doc := model.NewLivestockCreditLine(a, partner)
	if err := s.lines.Create(ctx, line, doc); err != nil {
		// A concurrent request may have registered it first (unique indexes):
		// answer as if this one had arrived second.
		if existing, findErr := s.existing(ctx, a, partner); existing != nil || findErr != nil {
			return existing, false, findErr
		}
		return nil, false, fmt.Errorf("storing the credit line: %w", err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, nil, "register_livestock_credit_line", "livestock_credit_line", line.ID.String(),
			fmt.Sprintf("%s activated a %s %s credit line on assessment %s (promissory note %s)",
				partner, line.Amount.StringFixed(2), line.Currency, line.AssessmentID, line.PromissoryNoteNumber))
	}
	return line, true, nil
}

// existing finds a line registered under any of the activation's identifiers.
// A replay returns it; different data under the same identifiers is a
// conflict.
func (s *LivestockCreditLineService) existing(ctx context.Context, a model.LivestockCreditActivation, partner string) (*model.LivestockCreditLine, error) {
	finders := []func() (*model.LivestockCreditLine, error){
		func() (*model.LivestockCreditLine, error) {
			return s.lines.FindByPromissoryNote(ctx, a.PromissoryNoteNumber)
		},
		func() (*model.LivestockCreditLine, error) {
			return s.lines.FindByPartnerReference(ctx, partner, a.PartnerReference)
		},
		func() (*model.LivestockCreditLine, error) { return s.lines.FindByAssessment(ctx, a.AssessmentID) },
	}
	for _, find := range finders {
		line, err := find()
		if err != nil {
			return nil, fmt.Errorf("looking up credit lines: %w", err)
		}
		if line == nil {
			continue
		}
		if line.Replays(a, partner) {
			return line, nil
		}
		return nil, line.ConflictWith(a, partner)
	}
	return nil, nil
}

// List returns a page of credit lines, most recently activated first, with
// the number of lines matching the filter. The page size defaults to
// DefaultCreditLinePageSize and is capped at MaxCreditLinePageSize.
func (s *LivestockCreditLineService) List(ctx context.Context, f port.LivestockCreditLineFilter) ([]model.LivestockCreditLine, int64, port.LivestockCreditLineFilter, error) {
	switch f.Status {
	case "", model.LivestockCreditLineActive:
	default:
		return nil, 0, f, ErrInvalidCreditLineStatus
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.Limit <= 0 {
		f.Limit = DefaultCreditLinePageSize
	}
	if f.Limit > MaxCreditLinePageSize {
		f.Limit = MaxCreditLinePageSize
	}
	lines, total, err := s.lines.List(ctx, f)
	if err != nil {
		return nil, 0, f, fmt.Errorf("listing credit lines: %w", err)
	}
	return lines, total, f, nil
}

// Get returns a credit line and the quota assessment it was granted on (nil
// if the assessment cannot be found, which the foreign key should prevent).
func (s *LivestockCreditLineService) Get(ctx context.Context, id uuid.UUID) (*model.LivestockCreditLine, *model.LivestockQuotaAssessment, error) {
	line, err := s.lines.FindByID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the credit line: %w", err)
	}
	if line == nil {
		return nil, nil, ErrCreditLineNotFound
	}
	assessment, err := s.assessments.FindByID(ctx, line.AssessmentID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the assessment: %w", err)
	}
	return line, assessment, nil
}

// PromissoryNote returns a line's signed promissory note after checking its
// bytes still hash to the SHA-256 registered with the line. Every access, and
// every failed integrity check, is recorded in the audit log under the viewer.
func (s *LivestockCreditLineService) PromissoryNote(ctx context.Context, id uuid.UUID, viewer *uuid.UUID) (*model.LivestockCreditLine, *model.LivestockCreditLineDocument, error) {
	line, err := s.lines.FindByID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the credit line: %w", err)
	}
	if line == nil {
		return nil, nil, ErrCreditLineNotFound
	}
	doc, err := s.lines.FindDocument(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the promissory note: %w", err)
	}
	if doc == nil {
		return nil, nil, ErrCreditLineNotFound
	}
	sum := sha256.Sum256(doc.Content)
	if actual := hex.EncodeToString(sum[:]); actual != line.DocumentSHA256 || int64(len(doc.Content)) != line.DocumentSize {
		s.record(ctx, viewer, "promissory_note_integrity_failed", line,
			fmt.Sprintf("promissory note %s does not match its registered SHA-256 (stored %s, computed %s)",
				line.PromissoryNoteNumber, line.DocumentSHA256, actual))
		return nil, nil, ErrPromissoryNoteCorrupted
	}
	s.record(ctx, viewer, "view_livestock_promissory_note", line,
		fmt.Sprintf("promissory note %s of credit line %s viewed", line.PromissoryNoteNumber, line.ID))
	return line, doc, nil
}

func (s *LivestockCreditLineService) record(ctx context.Context, viewer *uuid.UUID, action string, line *model.LivestockCreditLine, description string) {
	if s.audit != nil {
		s.audit.Record(ctx, viewer, action, "livestock_credit_line", line.ID.String(), description)
	}
}

// AsActivationRejection unwraps a rejection, if err is one.
func AsActivationRejection(err error) (*model.ActivationRejection, bool) {
	var r *model.ActivationRejection
	ok := errors.As(err, &r)
	return r, ok
}
