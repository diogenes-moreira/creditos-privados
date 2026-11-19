package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LivestockCreditLineStatus is where a rancher's livestock credit line stands.
// Lines are only registered as active for now: the partner (CuppoGan) activates
// them when the rancher signs the promissory note. Prestia will manage the
// credit from here (draws, payments, closing), adding statuses as it does.
type LivestockCreditLineStatus string

const (
	LivestockCreditLineActive LivestockCreditLineStatus = "active"
)

// MaxPromissoryNoteSize is the largest signed promissory note accepted. The
// pilot's notes, with the signature provider's audit page, are around 100 KB.
const MaxPromissoryNoteSize = 10 << 20

// PromissoryNoteContentType is the only document type accepted.
const PromissoryNoteContentType = "application/pdf"

// LivestockCreditLine is a rancher's credit line opened on a livestock quota:
// the partner that onboarded the rancher (CuppoGan) informs Prestia once the
// promissory note is signed. It references the quota assessment it was granted
// on, so the amount can always be traced back to the engine's decision.
//
// The signed promissory note is kept in LivestockCreditLineDocument, a separate
// table, so reading lines never loads the PDF.
type LivestockCreditLine struct {
	ID     uuid.UUID                 `gorm:"type:uuid;primaryKey"`
	Status LivestockCreditLineStatus `gorm:"type:varchar(20);not null"`

	// AssessmentID is the quota assessment the line was granted on. One
	// assessment opens at most one line.
	AssessmentID uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex"`
	Assessment   *LivestockQuotaAssessment `gorm:"foreignKey:AssessmentID"`

	NationalID  string `gorm:"type:varchar(30);index;not null"`
	IDType      string `gorm:"type:varchar(60)"`
	FullName    string `gorm:"type:varchar(200);not null"`
	MobilePhone string `gorm:"type:varchar(30)"`

	Amount   decimal.Decimal `gorm:"type:decimal(18,2);not null"`
	Currency string          `gorm:"type:varchar(3);not null"`

	// Partner is the service that informed the activation; PartnerReference its
	// own identifier for it (CuppoGan's onboarding ID). Unique together.
	Partner          string `gorm:"type:varchar(60);not null;uniqueIndex:idx_livestock_credit_lines_partner_ref"`
	PartnerReference string `gorm:"type:varchar(100);not null;uniqueIndex:idx_livestock_credit_lines_partner_ref"`

	// PromissoryNoteNumber is the partner's number for the note
	// (CPG-YYYY-NNNNNN). It identifies the activation: a replay with the same
	// number returns the line already registered.
	PromissoryNoteNumber string    `gorm:"type:varchar(40);not null;uniqueIndex"`
	SignedAt             time.Time `gorm:"not null"`
	ActivatedAt          time.Time `gorm:"not null"`

	// SignatureProvider and its identifiers, as the partner reports them
	// (e.g. Truora's validation and ZapSign's document), for tracing only.
	SignatureProvider     string `gorm:"type:varchar(60)"`
	SignatureDocumentID   string `gorm:"type:varchar(100)"`
	SignatureValidationID string `gorm:"type:varchar(100)"`

	// DocumentSHA256 (hex) and DocumentSize describe the stored PDF; the
	// partner's own copy is at PartnerArchiveURI (informative).
	DocumentSHA256    string `gorm:"type:char(64);not null"`
	DocumentSize      int64  `gorm:"not null"`
	PartnerArchiveURI string `gorm:"type:varchar(500)"`

	// Odoo is the line's link to its contact in TVGAN's Odoo, where the
	// credit is invoiced and collected, and the last credit snapshot.
	Odoo LivestockCreditLineOdoo `gorm:"embedded;embeddedPrefix:odoo_"`

	// ClientID and CreditLineID are the rancher and the approved credit line
	// in Prestia's own tables (clients, credit_lines): the activation enrolls
	// the rancher there, so they show up with every other client. Nil only on
	// a line registered before that, until the partner replays its activation.
	ClientID     *uuid.UUID `gorm:"type:uuid;index"`
	CreditLineID *uuid.UUID `gorm:"type:uuid;index"`

	// LoanID is the loan the partner opened on the quota (one per activation),
	// with the rate as the partner stated it: LoanAgreedRate in LoanRateType
	// (e.g. 26.01 TEA). The loan itself keeps the nominal equivalent.
	LoanID         *uuid.UUID          `gorm:"type:uuid;index"`
	LoanAgreedRate decimal.NullDecimal `gorm:"type:decimal(8,4)"`
	LoanRateType   string              `gorm:"type:varchar(20)"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// LivestockCreditLineDocument is the signed promissory note of a credit line,
// stored in Postgres (bytea). Prestia has no durable file store (the storage
// adapter writes to the container's disk), and the notes are small.
type LivestockCreditLineDocument struct {
	CreditLineID uuid.UUID `gorm:"type:uuid;primaryKey"`
	ContentType  string    `gorm:"type:varchar(100);not null"`
	SHA256       string    `gorm:"type:char(64);not null"`
	Size         int64     `gorm:"not null"`
	Content      []byte    `gorm:"type:bytea;not null"`
	CreatedAt    time.Time
}

// LivestockCreditActivation is what a partner informs when a rancher's credit
// line is activated, with the signed promissory note.
type LivestockCreditActivation struct {
	AssessmentID          uuid.UUID
	NationalID            string
	IDType                string
	FullName              string
	MobilePhone           string
	Amount                decimal.Decimal
	Currency              string
	PromissoryNoteNumber  string
	SignedAt              time.Time
	ActivatedAt           time.Time
	DocumentSHA256        string
	PartnerReference      string
	PartnerArchiveURI     string
	SignatureProvider     string
	SignatureDocumentID   string
	SignatureValidationID string

	// The rancher as Prestia's client. Optional: a partner that does not send
	// them gets the names split from FullName, a generic date of birth and an
	// e-mail made from the national ID (see LivestockClientEnrollment).
	FirstName   string
	LastName    string
	Email       string
	DateOfBirth *time.Time
	Address     string
	City        string
	Province    string

	Document            []byte
	DocumentContentType string
}

// ActivationRejection is why an activation cannot be registered. Code is
// stable, for the partner to act on; Kind tells the HTTP layer which status.
type ActivationRejection struct {
	Kind    ActivationRejectionKind
	Code    string
	Message string
}

type ActivationRejectionKind int

const (
	// ActivationInvalid is a malformed request (400).
	ActivationInvalid ActivationRejectionKind = iota
	// ActivationTooLarge is a document over MaxPromissoryNoteSize (413).
	ActivationTooLarge
	// ActivationUnprocessable is a well-formed request whose data does not
	// hold: not a PDF, wrong hash, or not matching the assessment (422).
	ActivationUnprocessable
	// ActivationConflict clashes with a line already registered (409).
	ActivationConflict
)

func (e *ActivationRejection) Error() string { return e.Code + ": " + e.Message }

func reject(kind ActivationRejectionKind, code, format string, args ...any) *ActivationRejection {
	return &ActivationRejection{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Validate checks the activation on its own: required fields and the signed
// promissory note (a PDF, within size, matching its declared SHA-256).
func (a *LivestockCreditActivation) Validate() error {
	a.normalize()
	switch {
	case a.AssessmentID == uuid.Nil:
		return reject(ActivationInvalid, "invalid_request", "assessmentId is required")
	case a.NationalID == "":
		return reject(ActivationInvalid, "invalid_request", "nationalId is required")
	case a.FullName == "":
		return reject(ActivationInvalid, "invalid_request", "fullName is required")
	case !a.Amount.IsPositive():
		return reject(ActivationInvalid, "invalid_request", "amount must be positive")
	case len(a.Currency) != 3:
		return reject(ActivationInvalid, "invalid_request", "currency must be an ISO 4217 code")
	case a.PromissoryNoteNumber == "" || len(a.PromissoryNoteNumber) > 40:
		return reject(ActivationInvalid, "invalid_request", "promissoryNoteNumber is required (at most 40 characters)")
	case a.PartnerReference == "" || len(a.PartnerReference) > 100:
		return reject(ActivationInvalid, "invalid_request", "partnerReference is required (at most 100 characters)")
	case a.SignedAt.IsZero():
		return reject(ActivationInvalid, "invalid_request", "signedAt is required")
	case a.ActivatedAt.IsZero():
		return reject(ActivationInvalid, "invalid_request", "activatedAt is required")
	case len(a.DocumentSHA256) != sha256.Size*2:
		return reject(ActivationInvalid, "invalid_request", "documentSha256 must be a hex SHA-256")
	}
	return a.validateDocument()
}

func (a *LivestockCreditActivation) normalize() {
	a.NationalID = strings.TrimSpace(a.NationalID)
	a.IDType = strings.TrimSpace(a.IDType)
	a.FullName = strings.TrimSpace(a.FullName)
	a.MobilePhone = strings.TrimSpace(a.MobilePhone)
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	a.PromissoryNoteNumber = strings.TrimSpace(a.PromissoryNoteNumber)
	a.PartnerReference = strings.TrimSpace(a.PartnerReference)
	a.DocumentSHA256 = strings.ToLower(strings.TrimSpace(a.DocumentSHA256))
	a.Amount = a.Amount.Round(2)
}

func (a *LivestockCreditActivation) validateDocument() error {
	if len(a.Document) == 0 {
		return reject(ActivationInvalid, "invalid_request", "the promissory note is required")
	}
	if len(a.Document) > MaxPromissoryNoteSize {
		return reject(ActivationTooLarge, "document_too_large", "the promissory note exceeds %d bytes", MaxPromissoryNoteSize)
	}
	mediaType, _, err := mime.ParseMediaType(a.DocumentContentType)
	if err != nil || mediaType != PromissoryNoteContentType || !bytes.HasPrefix(a.Document, []byte("%PDF-")) {
		return reject(ActivationUnprocessable, "not_a_pdf", "the promissory note must be a PDF (application/pdf)")
	}
	sum := sha256.Sum256(a.Document)
	if hex.EncodeToString(sum[:]) != a.DocumentSHA256 {
		return reject(ActivationUnprocessable, "sha256_mismatch", "the promissory note does not match documentSha256")
	}
	return nil
}

// CheckAgainst verifies the activation opens the quota the partner was
// granted: an assessment the same partner requested, for the same national ID,
// for its full quota in its currency.
func (a *LivestockCreditActivation) CheckAgainst(assessment *LivestockQuotaAssessment, partner string) error {
	switch {
	case assessment.RequestedBy != partner:
		return reject(ActivationUnprocessable, "assessment_of_another_partner", "the assessment was not requested by this partner")
	case assessment.NationalID != a.NationalID:
		return reject(ActivationUnprocessable, "national_id_mismatch", "the assessment belongs to another national ID")
	case !assessment.Quota.IsPositive():
		return reject(ActivationUnprocessable, "assessment_without_quota", "the assessment granted no quota")
	case assessment.Currency != a.Currency:
		return reject(ActivationUnprocessable, "currency_mismatch", "the assessment quota is in %s", assessment.Currency)
	case !assessment.Quota.Equal(a.Amount):
		return reject(ActivationUnprocessable, "amount_mismatch", "the amount must equal the assessment quota (%s)", assessment.Quota.StringFixed(2))
	}
	return nil
}

// NewLivestockCreditLine opens the active line an activation describes, with
// its promissory note. The activation must have been validated.
func NewLivestockCreditLine(a LivestockCreditActivation, partner string) (*LivestockCreditLine, *LivestockCreditLineDocument) {
	line := &LivestockCreditLine{
		ID:                    uuid.New(),
		Status:                LivestockCreditLineActive,
		AssessmentID:          a.AssessmentID,
		NationalID:            a.NationalID,
		IDType:                a.IDType,
		FullName:              a.FullName,
		MobilePhone:           a.MobilePhone,
		Amount:                a.Amount,
		Currency:              a.Currency,
		Partner:               partner,
		PartnerReference:      a.PartnerReference,
		PromissoryNoteNumber:  a.PromissoryNoteNumber,
		SignedAt:              a.SignedAt.UTC(),
		ActivatedAt:           a.ActivatedAt.UTC(),
		SignatureProvider:     a.SignatureProvider,
		SignatureDocumentID:   a.SignatureDocumentID,
		SignatureValidationID: a.SignatureValidationID,
		DocumentSHA256:        a.DocumentSHA256,
		DocumentSize:          int64(len(a.Document)),
		PartnerArchiveURI:     a.PartnerArchiveURI,
	}
	doc := &LivestockCreditLineDocument{
		CreditLineID: line.ID,
		ContentType:  PromissoryNoteContentType,
		SHA256:       a.DocumentSHA256,
		Size:         int64(len(a.Document)),
		Content:      a.Document,
	}
	return line, doc
}

// Replays tells whether an activation is a replay of the one that opened this
// line: the same partner, reference, note, assessment, rancher, amount and
// document. Anything else under the same identifiers is a conflict.
func (l *LivestockCreditLine) Replays(a LivestockCreditActivation, partner string) bool {
	return l.Partner == partner &&
		l.PartnerReference == a.PartnerReference &&
		l.PromissoryNoteNumber == a.PromissoryNoteNumber &&
		l.AssessmentID == a.AssessmentID &&
		l.NationalID == a.NationalID &&
		l.Amount.Equal(a.Amount) &&
		l.Currency == a.Currency &&
		l.DocumentSHA256 == a.DocumentSHA256
}

// ConflictWith explains why an activation clashes with this line.
func (l *LivestockCreditLine) ConflictWith(a LivestockCreditActivation, partner string) error {
	switch {
	case l.PromissoryNoteNumber == a.PromissoryNoteNumber:
		return reject(ActivationConflict, "promissory_note_already_registered",
			"promissory note %s is already registered with different data", a.PromissoryNoteNumber)
	case l.Partner == partner && l.PartnerReference == a.PartnerReference:
		return reject(ActivationConflict, "partner_reference_already_registered",
			"partner reference %s is already registered with different data", a.PartnerReference)
	default:
		return reject(ActivationConflict, "assessment_already_activated",
			"the assessment already opened another credit line")
	}
}
