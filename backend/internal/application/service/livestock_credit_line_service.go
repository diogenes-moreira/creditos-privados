package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
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

// AsActivationRejection unwraps a rejection, if err is one.
func AsActivationRejection(err error) (*model.ActivationRejection, bool) {
	var r *model.ActivationRejection
	ok := errors.As(err, &r)
	return r, ok
}
