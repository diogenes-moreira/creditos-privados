package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/gin-gonic/gin"
)

// maxActivationMetadata is the room left for the metadata part and the
// multipart framing on top of the promissory note.
const maxActivationMetadata = 1 << 20

// LivestockCreditLineHandler receives the credit lines partner services
// (CuppoGan) activate on a livestock quota.
type LivestockCreditLineHandler struct {
	service *service.LivestockCreditLineService
}

func NewLivestockCreditLineHandler(s *service.LivestockCreditLineService) *LivestockCreditLineHandler {
	return &LivestockCreditLineHandler{service: s}
}

// RegisterActivation godoc
// @Summary Register a livestock credit line activated by a partner
// @Description The partner informs that the rancher signed the promissory note and the quota is active. Prestia records the credit line with the signed PDF (registration only for now). The line must open the full quota of an assessment the same partner requested for the same national ID. Idempotent: replaying the same activation (same promissoryNoteNumber, partnerReference, assessment, national ID, amount and document) returns 200 with the line already registered; different data under the same promissory note, partner reference or assessment is 409. Error codes: invalid_request (400), document_too_large (413), not_a_pdf, sha256_mismatch, unknown_assessment, assessment_of_another_partner, national_id_mismatch, assessment_without_quota, currency_mismatch, amount_mismatch (422), promissory_note_already_registered, partner_reference_already_registered, assessment_already_activated (409).
// @Tags Livestock quota
// @Accept multipart/form-data
// @Produce json
// @Param metadata formData string true "JSON, see dto.LivestockCreditActivationMetadata"
// @Param promissoryNote formData file true "Signed promissory note (application/pdf, at most 10 MB)"
// @Success 201 {object} dto.LivestockCreditLineResponse "Registered"
// @Success 200 {object} dto.LivestockCreditLineResponse "Already registered (replay)"
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 409 {object} dto.CodedErrorResponse
// @Failure 413 {object} dto.CodedErrorResponse
// @Failure 422 {object} dto.CodedErrorResponse
// @Security ServiceKey
// @Router /livestock-quotas/activations [post]
func (h *LivestockCreditLineHandler) RegisterActivation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, model.MaxPromissoryNoteSize+maxActivationMetadata)
	form, err := c.MultipartForm()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			codedError(c, http.StatusRequestEntityTooLarge, "document_too_large", "the request exceeds the size limit")
			return
		}
		codedError(c, http.StatusBadRequest, "invalid_request", "expected multipart/form-data with metadata and promissoryNote")
		return
	}

	raw, err := metadataPart(form)
	if err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var metadata dto.LivestockCreditActivationMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "metadata is not valid JSON")
		return
	}
	files := form.File["promissoryNote"]
	if len(files) != 1 {
		codedError(c, http.StatusBadRequest, "invalid_request", "exactly one promissoryNote part is required")
		return
	}
	document, err := readPart(files[0], model.MaxPromissoryNoteSize)
	if err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the promissoryNote part cannot be read")
		return
	}

	activation, err := dto.ToLivestockCreditActivation(metadata, document, files[0].Header.Get("Content-Type"))
	if err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	line, created, err := h.service.RegisterActivation(c.Request.Context(), activation, c.GetString("serviceName"))
	if rejection, ok := service.AsActivationRejection(err); ok {
		codedError(c, rejectionStatus(rejection.Kind), rejection.Code, rejection.Message)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, dto.ToLivestockCreditLineResponse(line))
}

// metadataPart reads the metadata as a plain field or as a JSON file part.
func metadataPart(form *multipart.Form) ([]byte, error) {
	if values := form.Value["metadata"]; len(values) == 1 {
		return []byte(values[0]), nil
	}
	if files := form.File["metadata"]; len(files) == 1 {
		return readPart(files[0], maxActivationMetadata)
	}
	return nil, errors.New("exactly one metadata part is required")
}

// readPart reads one part, reading one byte past limit so the size check
// downstream still sees a part that is too large.
func readPart(h *multipart.FileHeader, limit int64) ([]byte, error) {
	f, err := h.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit+1))
}

func rejectionStatus(kind model.ActivationRejectionKind) int {
	switch kind {
	case model.ActivationTooLarge:
		return http.StatusRequestEntityTooLarge
	case model.ActivationUnprocessable:
		return http.StatusUnprocessableEntity
	case model.ActivationConflict:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func codedError(c *gin.Context, status int, code, message string) {
	c.JSON(status, dto.CodedErrorResponse{Code: code, Error: message})
}

// OpenLoan godoc
// @Summary Open the loan of an activated livestock quota
// @Description The partner opens the loan of a quota it activated, on the terms it agreed with the rancher: total, interest rate (TEA — tasa efectiva anual — or NOMINAL_ANNUAL, Prestia's monthly rate × 12), monthly installments and amortization system (french or german). Prestia converts the rate to its nominal annual convention, adopts the terms on the rancher's credit line, computes the schedule (first installment a month after opening, no VAT on interest) and records the disbursement. One loan per activation: replaying the same request returns 200 with the loan; other terms are 409. Error codes: missing_partner_reference, invalid_total, invalid_rate, invalid_rate_type, invalid_installments, invalid_system (400), unknown_activation, exceeds_credit_line (422), not_enrolled, loan_exists (409).
// @Tags Livestock quota
// @Accept json
// @Produce json
// @Param request body dto.LivestockLoanRequest true "Loan terms"
// @Success 201 {object} dto.LivestockLoanResponse "Opened"
// @Success 200 {object} dto.LivestockLoanResponse "Already opened (replay)"
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 409 {object} dto.CodedErrorResponse
// @Failure 422 {object} dto.CodedErrorResponse
// @Security ServiceKey
// @Router /livestock-quotas/loans [post]
func (h *LivestockCreditLineHandler) OpenLoan(c *gin.Context) {
	var req dto.LivestockLoanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	loan, line, created, err := h.service.OpenLoan(c.Request.Context(), c.GetString("serviceName"), dto.ToLivestockLoanRequest(req))
	if rejection, ok := service.AsActivationRejection(err); ok {
		codedError(c, rejectionStatus(rejection.Kind), rejection.Code, rejection.Message)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: err.Error()})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, dto.ToLivestockLoanResponse(loan, line))
}
