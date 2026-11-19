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
	"github.com/shopspring/decimal"
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
	enrollment  *clientEnrollment
}

// LivestockCreditTerms are the terms of the CreditLine an activation opens in
// Prestia: the interest rate and the most installments a purchase can take.
type LivestockCreditTerms struct {
	InterestRate    decimal.Decimal
	MaxInstallments int
}

// clientEnrollment is what enrolling a rancher as Prestia's client needs.
type clientEnrollment struct {
	users   port.UserRepository
	clients port.ClientRepository
	auth    port.AuthService
	country string
	terms   LivestockCreditTerms
}

func NewLivestockCreditLineService(lines port.LivestockCreditLineRepository, assessments port.LivestockQuotaRepository) *LivestockCreditLineService {
	return &LivestockCreditLineService{lines: lines, assessments: assessments}
}

// WithClients enrolls the rancher of every activation in Prestia's own
// tables: a Client (with its User and current account) unless one with that
// national ID exists, and an approved CreditLine for the quota. It also
// enrolls the rancher of a line registered before, when its activation is
// replayed.
func (s *LivestockCreditLineService) WithClients(users port.UserRepository, clients port.ClientRepository, auth port.AuthService, country string, terms LivestockCreditTerms) *LivestockCreditLineService {
	s.enrollment = &clientEnrollment{users: users, clients: clients, auth: auth, country: country, terms: terms}
	return s
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
		if err == nil && line.ClientID == nil && s.enrollment != nil {
			err = s.linkClient(ctx, a, line)
		}
		if err != nil {
			return nil, false, err
		}
		return line, false, nil
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
	var enrollment *model.LivestockClientEnrollment
	if s.enrollment != nil {
		if enrollment, err = s.enroll(ctx, a); err != nil {
			return nil, false, err
		}
	}
	if err := s.lines.Create(ctx, line, doc, enrollment); err != nil {
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
		s.recordEnrollment(ctx, line, enrollment, partner)
	}
	return line, true, nil
}

// linkClient enrolls the rancher of a line registered before activations did.
func (s *LivestockCreditLineService) linkClient(ctx context.Context, a model.LivestockCreditActivation, line *model.LivestockCreditLine) error {
	enrollment, err := s.enroll(ctx, a)
	if err != nil {
		return err
	}
	if err := s.lines.LinkClient(ctx, line, enrollment); err != nil {
		return fmt.Errorf("enrolling the rancher of credit line %s: %w", line.ID, err)
	}
	s.recordEnrollment(ctx, line, enrollment, line.Partner)
	return nil
}

// enroll builds the rancher's client (unless Prestia has one with that
// national ID) and the approved credit line of the quota. Nothing is stored.
func (s *LivestockCreditLineService) enroll(ctx context.Context, a model.LivestockCreditActivation) (*model.LivestockClientEnrollment, error) {
	e := s.enrollment
	out := &model.LivestockClientEnrollment{}
	if client, err := e.clients.FindByDNI(ctx, a.NationalID); err == nil && client != nil {
		out.Client = client
	} else {
		nit, err := model.NaturalPersonNIT(a.NationalID)
		if err != nil {
			return nil, invalidClient(err)
		}
		first, last := a.ClientNames()
		dob, generic := a.ClientDateOfBirth()
		client, err := model.NewClient(uuid.Nil, first, last, a.NationalID, nit, dob, a.MobilePhone,
			a.Address, a.City, a.Province, "Colombia", false, e.country)
		if err != nil {
			return nil, invalidClient(err)
		}
		if generic {
			client.SetComments(model.GenericDateOfBirthComment)
		}
		email := s.clientEmail(ctx, a)
		authUser, err := e.auth.CreateUser(ctx, email, "")
		if err != nil {
			return nil, fmt.Errorf("creating the client's sign-in: %w", err)
		}
		user, err := model.NewUser(authUser.UID, email, model.RoleClient)
		if err != nil {
			return nil, invalidClient(err)
		}
		user.SetPhone(a.MobilePhone)
		client.UserID = user.ID
		out.User, out.Client, out.Account = user, client, model.NewCurrentAccount(client.ID)
	}
	creditLine, err := model.NewCreditLine(out.Client.ID, a.Amount, e.terms.InterestRate, e.terms.MaxInstallments, false)
	if err != nil {
		return nil, fmt.Errorf("the configured livestock credit terms are invalid: %w", err)
	}
	approvedAt := a.ActivatedAt.UTC()
	creditLine.Status, creditLine.ApprovedAt = model.CreditLineApproved, &approvedAt
	out.CreditLine = creditLine
	return out, nil
}

// clientEmail is the e-mail sent, unless missing or another user's; then
// <national ID>@cuppogan.com.
func (s *LivestockCreditLineService) clientEmail(ctx context.Context, a model.LivestockCreditActivation) string {
	if email := a.ClientEmail(); email != "" {
		if u, err := s.enrollment.users.FindByEmail(ctx, email); err != nil || u == nil {
			return email
		}
	}
	return a.RegistryEmail()
}

func (s *LivestockCreditLineService) recordEnrollment(ctx context.Context, line *model.LivestockCreditLine, e *model.LivestockClientEnrollment, partner string) {
	if s.audit == nil || e == nil {
		return
	}
	if e.NewClient() {
		s.audit.Record(ctx, nil, "register", "client", e.Client.ID.String(),
			fmt.Sprintf("Client %s registered by %s (livestock credit line %s)", e.Client.FullName(), partner, line.ID))
	}
	s.audit.Record(ctx, nil, "approve", "credit_line", e.CreditLine.ID.String(),
		fmt.Sprintf("Credit line of %s %s approved by %s's activation (promissory note %s)",
			e.CreditLine.MaxAmount.StringFixed(2), line.Currency, partner, line.PromissoryNoteNumber))
}

func invalidClient(err error) error {
	return &model.ActivationRejection{Kind: model.ActivationUnprocessable, Code: "invalid_client", Message: err.Error()}
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
