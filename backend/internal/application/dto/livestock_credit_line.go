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

// LivestockCreditLineSearchRequest filters the back office's credit line list.
// The national ID travels in the body, not the URL, so it stays out of request
// logs (as in the quota lookup).
type LivestockCreditLineSearchRequest struct {
	// NationalID matches exactly; empty lists every line.
	NationalID string `json:"nationalId" example:"17788090"`
	// Status filters by status (active); empty lists every status.
	Status string `json:"status" example:"active"`
	Offset int    `json:"offset"`
	// Limit defaults to 20, at most 100.
	Limit int `json:"limit" example:"20"`
}

// LivestockCreditLineListQuery is the query of the unfiltered list.
type LivestockCreditLineListQuery struct {
	Status string `form:"status"`
	Offset int    `form:"offset"`
	Limit  int    `form:"limit"`
}

// LivestockCreditLineSummary is a credit line as the back office lists it:
// metadata only, never the promissory note's bytes.
type LivestockCreditLineSummary struct {
	ID           string `json:"id"`
	Status       string `json:"status" example:"active"`
	AssessmentID string `json:"assessmentId"`

	NationalID  string `json:"nationalId"`
	IDType      string `json:"idType" example:"CÉDULA DE CIUDADANÍA"`
	FullName    string `json:"fullName"`
	MobilePhone string `json:"mobilePhone"`

	Amount   string `json:"amount" example:"1000000.00"`
	Currency string `json:"currency" example:"COP"`

	Partner              string    `json:"partner" example:"cuppogan"`
	PartnerReference     string    `json:"partnerReference"`
	PromissoryNoteNumber string    `json:"promissoryNoteNumber" example:"CPG-2026-000002"`
	SignedAt             time.Time `json:"signedAt"`
	ActivatedAt          time.Time `json:"activatedAt"`
	CreatedAt            time.Time `json:"createdAt"`

	DocumentSHA256 string `json:"documentSha256"`
	DocumentSize   int64  `json:"documentSize"`
}

// LivestockCreditLinePage is a page of credit lines.
type LivestockCreditLinePage struct {
	Data   []LivestockCreditLineSummary `json:"data"`
	Total  int64                        `json:"total"`
	Offset int                          `json:"offset"`
	Limit  int                          `json:"limit"`
}

// LivestockCreditLineAssessmentSummary is the quota assessment a line was
// granted on, as stored when the partner asked for the quota.
type LivestockCreditLineAssessmentSummary struct {
	ID       string `json:"id"`
	Quota    string `json:"quota" example:"1000000.00"`
	Currency string `json:"currency" example:"COP"`
	// Strategy is how the quota was decided: tiers or engine.
	Strategy string `json:"strategy" example:"tiers"`
	// Tier is the tier code (tier1, tier2, tier3, notEligible) and TierLabel
	// its name; both empty under the engine strategy.
	Tier              string `json:"tier,omitempty" example:"tier2"`
	TierLabel         string `json:"tierLabel,omitempty" example:"Escalón 2"`
	TierPolicyVersion string `json:"tierPolicyVersion,omitempty" example:"v1"`
	// EngineQuota is what the individual engine would have granted.
	EngineQuota string `json:"engineQuota" example:"2350000.00"`
	// ActiveCeiling is the ceiling that bound the engine's quota, or the tier
	// label under the tiers strategy.
	ActiveCeiling string    `json:"activeCeiling"`
	Score         int       `json:"score"`
	Grade         string    `json:"grade"`
	CreatedAt     time.Time `json:"createdAt"`
}

// LivestockCreditLineDetail is one credit line with the signature trace and
// the assessment it was granted on.
type LivestockCreditLineDetail struct {
	LivestockCreditLineSummary
	SignatureProvider     string `json:"signatureProvider" example:"truora-zapsign"`
	SignatureDocumentID   string `json:"signatureDocumentId"`
	SignatureValidationID string `json:"signatureValidationId"`
	// PartnerArchiveURI is where the partner keeps its own copy (informative).
	PartnerArchiveURI string    `json:"partnerArchiveUri"`
	UpdatedAt         time.Time `json:"updatedAt"`
	// Assessment is absent only if the stored assessment cannot be read.
	Assessment *LivestockCreditLineAssessmentSummary `json:"assessment,omitempty"`
}

func ToLivestockCreditLineSummary(l *model.LivestockCreditLine) LivestockCreditLineSummary {
	return LivestockCreditLineSummary{
		ID:                   l.ID.String(),
		Status:               string(l.Status),
		AssessmentID:         l.AssessmentID.String(),
		NationalID:           l.NationalID,
		IDType:               l.IDType,
		FullName:             l.FullName,
		MobilePhone:          l.MobilePhone,
		Amount:               l.Amount.StringFixed(2),
		Currency:             l.Currency,
		Partner:              l.Partner,
		PartnerReference:     l.PartnerReference,
		PromissoryNoteNumber: l.PromissoryNoteNumber,
		SignedAt:             l.SignedAt,
		ActivatedAt:          l.ActivatedAt,
		CreatedAt:            l.CreatedAt,
		DocumentSHA256:       l.DocumentSHA256,
		DocumentSize:         l.DocumentSize,
	}
}

func ToLivestockCreditLinePage(lines []model.LivestockCreditLine, total int64, offset, limit int) LivestockCreditLinePage {
	data := make([]LivestockCreditLineSummary, len(lines))
	for i := range lines {
		data[i] = ToLivestockCreditLineSummary(&lines[i])
	}
	return LivestockCreditLinePage{Data: data, Total: total, Offset: offset, Limit: limit}
}

func ToLivestockCreditLineDetail(l *model.LivestockCreditLine, a *model.LivestockQuotaAssessment) LivestockCreditLineDetail {
	d := LivestockCreditLineDetail{
		LivestockCreditLineSummary: ToLivestockCreditLineSummary(l),
		SignatureProvider:          l.SignatureProvider,
		SignatureDocumentID:        l.SignatureDocumentID,
		SignatureValidationID:      l.SignatureValidationID,
		PartnerArchiveURI:          l.PartnerArchiveURI,
		UpdatedAt:                  l.UpdatedAt,
	}
	if a == nil {
		return d
	}
	// Rows stored before strategies existed are empty and were all decided
	// by the engine.
	strategy := a.Strategy
	if strategy == "" {
		strategy = string(model.QuotaStrategyEngine)
	}
	s := &LivestockCreditLineAssessmentSummary{
		ID:            a.ID.String(),
		Quota:         a.Quota.StringFixed(2),
		Currency:      a.Currency,
		Strategy:      strategy,
		EngineQuota:   a.EngineQuota.StringFixed(2),
		ActiveCeiling: a.ActiveCeiling,
		Score:         a.Score,
		Grade:         a.Grade,
		CreatedAt:     a.CreatedAt,
	}
	if strategy == string(model.QuotaStrategyTiers) {
		// ApplyTier stores the tier's label as the active ceiling.
		s.Tier, s.TierLabel, s.TierPolicyVersion = a.Tier, a.ActiveCeiling, a.TierPolicyVersion
	}
	d.Assessment = s
	return d
}
