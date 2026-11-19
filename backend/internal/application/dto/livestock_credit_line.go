package dto

import (
	"fmt"
	"strings"
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

	// The rancher as Prestia's client (all optional). Without names, FullName
	// is split; without a date of birth (YYYY-MM-DD) a generic one is used and
	// the client's comments say so; without an e-mail, or with one another
	// user has, it is <nationalId>@cuppogan.com.
	FirstName   string `json:"firstName,omitempty"`
	LastName    string `json:"lastName,omitempty"`
	Email       string `json:"email,omitempty"`
	DateOfBirth string `json:"dateOfBirth,omitempty" example:"1975-04-30"`
	// Address, City and Province (department) of the rancher's main holding.
	Address  string `json:"address,omitempty"`
	City     string `json:"city,omitempty"`
	Province string `json:"province,omitempty"`
}

// ToLivestockCreditActivation maps the metadata and the document to the domain.
func ToLivestockCreditActivation(m LivestockCreditActivationMetadata, document []byte, contentType string) (model.LivestockCreditActivation, error) {
	id, err := uuid.Parse(m.AssessmentID)
	if err != nil {
		return model.LivestockCreditActivation{}, fmt.Errorf("assessmentId must be a UUID")
	}
	var dob *time.Time
	if m.DateOfBirth != "" {
		d, err := time.Parse("2006-01-02", m.DateOfBirth)
		if err != nil {
			return model.LivestockCreditActivation{}, fmt.Errorf("dateOfBirth must be YYYY-MM-DD")
		}
		dob = &d
	}
	return model.LivestockCreditActivation{
		FirstName:             m.FirstName,
		LastName:              m.LastName,
		Email:                 m.Email,
		DateOfBirth:           dob,
		Address:               m.Address,
		City:                  m.City,
		Province:              m.Province,
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
	// ClientID and CreditLineID are the rancher and the credit line in
	// Prestia's own tables.
	ClientID     *string `json:"clientId,omitempty"`
	CreditLineID *string `json:"creditLineId,omitempty"`
}

func uuidString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
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
		ClientID:             uuidString(l.ClientID),
		CreditLineID:         uuidString(l.CreditLineID),
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

	// Odoo is the line's link to TVGAN's Odoo and the overdue days and block
	// of its last credit snapshot.
	Odoo LivestockCreditLineOdooSummary `json:"odoo"`
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
	// ClientID is the rancher in Prestia's clients (absent until enrolled).
	ClientID *string `json:"clientId,omitempty"`
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
		Odoo:                 ToLivestockCreditLineOdooSummary(l.Odoo),
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
		ClientID:                   uuidString(l.ClientID),
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

// LivestockLoanRequest opens the loan of an activated quota.
type LivestockLoanRequest struct {
	// PartnerReference is the activation's partnerReference (CuppoGan's onboarding ID).
	PartnerReference string `json:"partnerReference"`
	// Total is the loan's principal; at most the credit line's available amount.
	Total decimal.Decimal `json:"total" swaggertype:"string" example:"1299155.00"`
	// Rate is the interest rate as agreed, in RateType: TEA (tasa efectiva
	// anual) or NOMINAL_ANNUAL (Prestia's monthly rate × 12).
	Rate     decimal.Decimal `json:"rate" swaggertype:"string" example:"26.01"`
	RateType string          `json:"rateType" example:"TEA"`
	// Installments are monthly; the first is due a month after the loan opens.
	Installments int `json:"installments" example:"12"`
	// System is the amortization system: french (fixed payment) or german (fixed capital).
	System string `json:"system" example:"french"`
}

// ToLivestockLoanRequest maps the request; validation is the service's.
func ToLivestockLoanRequest(r LivestockLoanRequest) model.LivestockLoanRequest {
	return model.LivestockLoanRequest{
		PartnerReference: r.PartnerReference,
		Total:            r.Total,
		Rate:             r.Rate,
		RateType:         model.RateType(strings.ToUpper(strings.TrimSpace(r.RateType))),
		Installments:     r.Installments,
		System:           model.AmortizationType(strings.ToLower(strings.TrimSpace(r.System))),
	}
}

// LivestockLoanInstallment is one installment of the schedule.
type LivestockLoanInstallment struct {
	Number   int       `json:"number"`
	DueDate  time.Time `json:"dueDate"`
	Capital  string    `json:"capital" example:"97163.21"`
	Interest string    `json:"interest" example:"25271.86"`
	Total    string    `json:"total" example:"122435.07"`
}

// LivestockLoanResponse is the loan Prestia opened, with its schedule.
type LivestockLoanResponse struct {
	LoanID       string `json:"loanId"`
	CreditLineID string `json:"creditLineId"`
	ClientID     string `json:"clientId"`
	Status       string `json:"status" example:"active"`
	Total        string `json:"total" example:"1299155.00"`
	Currency     string `json:"currency" example:"COP"`
	// Rate and RateType as agreed; NominalAnnualRate is what Prestia computes
	// with (monthly rate × 12) and MonthlyRate the monthly percentage.
	Rate              string                     `json:"rate" example:"26.01"`
	RateType          string                     `json:"rateType" example:"TEA"`
	NominalAnnualRate string                     `json:"nominalAnnualRate" example:"23.3433"`
	MonthlyRate       string                     `json:"monthlyRate" example:"1.945275"`
	Installments      int                        `json:"installments" example:"12"`
	System            string                     `json:"system" example:"french"`
	DisbursedAt       *time.Time                 `json:"disbursedAt,omitempty"`
	TotalToPay        string                     `json:"totalToPay" example:"1469220.84"`
	Schedule          []LivestockLoanInstallment `json:"schedule"`
}

func ToLivestockLoanResponse(loan *model.Loan, line *model.LivestockCreditLine) LivestockLoanResponse {
	out := LivestockLoanResponse{
		LoanID:            loan.ID.String(),
		CreditLineID:      loan.CreditLineID.String(),
		ClientID:          loan.ClientID.String(),
		Status:            string(loan.Status),
		Total:             loan.Principal.StringFixed(2),
		Currency:          line.Currency,
		RateType:          line.LoanRateType,
		NominalAnnualRate: loan.InterestRate.StringFixed(4),
		MonthlyRate:       loan.InterestRate.Div(decimal.NewFromInt(12)).StringFixed(6),
		Installments:      loan.NumInstallments,
		System:            string(loan.AmortizationType),
		DisbursedAt:       loan.DisbursedAt,
		Schedule:          []LivestockLoanInstallment{},
	}
	if line.LoanAgreedRate.Valid {
		out.Rate = line.LoanAgreedRate.Decimal.String()
	}
	total := decimal.Zero
	for _, i := range loan.Installments {
		total = total.Add(i.TotalAmount)
		out.Schedule = append(out.Schedule, LivestockLoanInstallment{Number: i.Number, DueDate: i.DueDate,
			Capital: i.CapitalAmount.StringFixed(2), Interest: i.InterestAmount.StringFixed(2), Total: i.TotalAmount.StringFixed(2)})
	}
	out.TotalToPay = total.StringFixed(2)
	return out
}
