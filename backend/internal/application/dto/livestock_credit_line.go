package dto

import (
	"fmt"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LivestockCreditActivationMetadata is the "metadata" part of a credit line
// activation: who the rancher is, the quota opened and the signed promissory
// note sent in the "promissoryNote" part.
type LivestockCreditActivationMetadata struct {
	// AssessmentID is the quota assessment the line opens (the id Prestia
	// returned when the partner asked for the quota).
	AssessmentID string `json:"assessmentId" example:"0b8f6a9e-6d1f-4a51-9d5e-1f2a3b4c5d6e"`
	NationalID   string `json:"nationalId"`
	// IDType is the document type as the RUV names it, e.g. "CÉDULA DE CIUDADANÍA".
	IDType      string `json:"idType"`
	FullName    string `json:"fullName"`
	MobilePhone string `json:"mobilePhone"`
	// Amount must equal the assessment's quota.
	Amount   decimal.Decimal `json:"amount" swaggertype:"string" example:"1000000.00"`
	Currency string          `json:"currency" example:"COP"`
	// PromissoryNoteNumber identifies the activation (CPG-YYYY-NNNNNN).
	PromissoryNoteNumber string    `json:"promissoryNoteNumber" example:"CPG-2026-000002"`
	SignedAt             time.Time `json:"signedAt"`
	ActivatedAt          time.Time `json:"activatedAt"`
	// DocumentSHA256 is the hex SHA-256 of the promissoryNote part.
	DocumentSHA256 string `json:"documentSha256"`
	// PartnerReference is the partner's id for the activation (CuppoGan's onboarding ID).
	PartnerReference string `json:"partnerReference"`
	// ArchiveURI is where the partner keeps its own copy (informative).
	ArchiveURI            string `json:"archiveUri"`
	SignatureProvider     string `json:"signatureProvider" example:"truora-zapsign"`
	SignatureDocumentID   string `json:"signatureDocumentId"`
	SignatureValidationID string `json:"signatureValidationId"`
}

// ToLivestockCreditActivation maps the metadata and the document to the domain.
func ToLivestockCreditActivation(m LivestockCreditActivationMetadata, document []byte, contentType string) (model.LivestockCreditActivation, error) {
	id, err := uuid.Parse(m.AssessmentID)
	if err != nil {
		return model.LivestockCreditActivation{}, fmt.Errorf("assessmentId must be a UUID")
	}
	return model.LivestockCreditActivation{
		AssessmentID:          id,
		NationalID:            m.NationalID,
		IDType:                m.IDType,
		FullName:              m.FullName,
		MobilePhone:           m.MobilePhone,
		Amount:                m.Amount,
		Currency:              m.Currency,
		PromissoryNoteNumber:  m.PromissoryNoteNumber,
		SignedAt:              m.SignedAt,
		ActivatedAt:           m.ActivatedAt,
		DocumentSHA256:        m.DocumentSHA256,
		PartnerReference:      m.PartnerReference,
		PartnerArchiveURI:     m.ArchiveURI,
		SignatureProvider:     m.SignatureProvider,
		SignatureDocumentID:   m.SignatureDocumentID,
		SignatureValidationID: m.SignatureValidationID,
		Document:              document,
		DocumentContentType:   contentType,
	}, nil
}

// LivestockCreditLineResponse is the registered line, without the rancher's
// personal data (the partner sent it).
type LivestockCreditLineResponse struct {
	ID                   string    `json:"id"`
	Status               string    `json:"status" example:"active"`
	AssessmentID         string    `json:"assessmentId"`
	PartnerReference     string    `json:"partnerReference"`
	PromissoryNoteNumber string    `json:"promissoryNoteNumber"`
	Amount               string    `json:"amount" example:"1000000.00"`
	Currency             string    `json:"currency"`
	DocumentSHA256       string    `json:"documentSha256"`
	DocumentSize         int64     `json:"documentSize"`
	ActivatedAt          time.Time `json:"activatedAt"`
	RegisteredAt         time.Time `json:"registeredAt"`
}

func ToLivestockCreditLineResponse(l *model.LivestockCreditLine) LivestockCreditLineResponse {
	return LivestockCreditLineResponse{
		ID:                   l.ID.String(),
		Status:               string(l.Status),
		AssessmentID:         l.AssessmentID.String(),
		PartnerReference:     l.PartnerReference,
		PromissoryNoteNumber: l.PromissoryNoteNumber,
		Amount:               l.Amount.StringFixed(2),
		Currency:             l.Currency,
		DocumentSHA256:       l.DocumentSHA256,
		DocumentSize:         l.DocumentSize,
		ActivatedAt:          l.ActivatedAt,
		RegisteredAt:         l.CreatedAt,
	}
}

// CodedErrorResponse is an error with a stable code a partner can act on.
type CodedErrorResponse struct {
	Code  string `json:"code" example:"amount_mismatch"`
	Error string `json:"error"`
}
