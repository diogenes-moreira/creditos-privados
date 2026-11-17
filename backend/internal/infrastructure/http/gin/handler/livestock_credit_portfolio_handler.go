package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// LivestockCreditPortfolioHandler serves the portfolio (cartera) of the
// livestock credit lines on TVGAN's Odoo to the back office.
type LivestockCreditPortfolioHandler struct {
	service *service.LivestockCreditPortfolioService
}

func NewLivestockCreditPortfolioHandler(s *service.LivestockCreditPortfolioService) *LivestockCreditPortfolioHandler {
	return &LivestockCreditPortfolioHandler{service: s}
}

// RegisterRoutes mounts the portfolio routes on a group that already
// authenticates and checks model.CreditPortfolioRoles.
func (h *LivestockCreditPortfolioHandler) RegisterRoutes(g gin.IRoutes) {
	g.GET("/livestock-credit-lines/:id/portfolio", h.GetPortfolio)
	g.POST("/livestock-credit-lines/:id/odoo/sync", h.SyncFromOdoo)
	g.PUT("/livestock-credit-lines/:id/odoo/partner", h.LinkOdooPartner)
	g.POST("/livestock-credit-lines/:id/payments", h.RegisterPayment)
	g.POST("/livestock-credit-lines/:id/block", h.SetBlocked)
	g.GET("/livestock-credit-portfolio/banks", h.ListBanks)
}

// GetPortfolio godoc
// @Summary Get a credit line's portfolio (Odoo)
// @Description What Prestia last read from TVGAN's Odoo for the line: the link to the Odoo contact, the credit snapshot Odoo computes (approved, used, available, overdue days, can buy and why not), the quota invoices with their installments (open ones first, by the due date of their oldest unpaid installment), the payments registered from Prestia and the block history. It does not call Odoo. Advisors and administrators.
// @Tags Livestock credit portfolio
// @Produce json
// @Param id path string true "Credit line UUID"
// @Success 200 {object} dto.LivestockCreditPortfolioResponse
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/portfolio [get]
func (h *LivestockCreditPortfolioHandler) GetPortfolio(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	p, err := h.service.Portfolio(c.Request.Context(), id)
	if err != nil {
		portfolioError(c, err)
		return
	}
	c.JSON(http.StatusOK, toPortfolioResponse(p))
}

// SyncFromOdoo godoc
// @Summary Update a credit line from Odoo
// @Description Reads the line's Odoo contact now: links it by national ID if it is not linked yet (one match links it, none leaves it not_in_odoo, several leave it ambiguous for staff to pick), then stores the credit snapshot and every quota invoice. Odoo failures are kept as the line's last sync error. Advisors and administrators.
// @Tags Livestock credit portfolio
// @Produce json
// @Param id path string true "Credit line UUID"
// @Success 200 {object} dto.LivestockCreditPortfolioResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Failure 422 {object} dto.PortfolioErrorResponse
// @Failure 503 {object} dto.PortfolioErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/odoo/sync [post]
func (h *LivestockCreditPortfolioHandler) SyncFromOdoo(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	p, err := h.service.Sync(c.Request.Context(), id)
	if err != nil {
		portfolioError(c, err)
		return
	}
	c.JSON(http.StatusOK, toPortfolioResponse(p))
}

// LinkOdooPartner godoc
// @Summary Link a credit line to an Odoo contact
// @Description For a national ID repeated in Odoo (ambiguous): links the line to the contact staff picks. Odoo is read to check the contact's document is the line's national ID (or that Odoo's search by it returned the contact); then the snapshot and invoices are loaded. Advisors and administrators.
// @Tags Livestock credit portfolio
// @Accept json
// @Produce json
// @Param id path string true "Credit line UUID"
// @Param request body dto.LinkOdooPartnerRequest true "Odoo contact id"
// @Success 200 {object} dto.LivestockCreditPortfolioResponse
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Failure 409 {object} dto.PortfolioErrorResponse
// @Failure 422 {object} dto.PortfolioErrorResponse
// @Failure 503 {object} dto.PortfolioErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/odoo/partner [put]
func (h *LivestockCreditPortfolioHandler) LinkOdooPartner(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	var req dto.LinkOdooPartnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the body must be {\"partnerId\": <Odoo contact id>}")
		return
	}
	p, err := h.service.LinkPartner(c.Request.Context(), id, req.PartnerID, actorOf(c))
	if err != nil {
		portfolioError(c, err)
		return
	}
	c.JSON(http.StatusOK, toPortfolioResponse(p))
}

// RegisterPayment godoc
// @Summary Register a rancher's payment in Odoo
// @Description Validates the payment (amount above zero and at most the invoice's balance, read from Odoo now; date not in the future, today when empty; a bank Odoo accepts; a reference not used by another payment) and sends it to Odoo, which applies it to the invoice's oldest installment. 201: registered. 200: Odoo already had that reference (status already_registered, nothing duplicated) or a double submission of a stored payment (replayed). 422 odoo_rejected: Odoo refused it; it is stored as failed with Odoo's code. 503: Odoo unavailable; nothing stored, retry with the same reference. Advisors and administrators.
// @Tags Livestock credit portfolio
// @Accept json
// @Produce json
// @Param id path string true "Credit line UUID"
// @Param request body dto.RegisterLivestockCreditPaymentRequest true "Payment"
// @Success 201 {object} dto.LivestockCreditPaymentResult
// @Success 200 {object} dto.LivestockCreditPaymentResult
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Failure 409 {object} dto.PortfolioErrorResponse
// @Failure 422 {object} dto.PortfolioErrorResponse
// @Failure 503 {object} dto.PortfolioErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/payments [post]
func (h *LivestockCreditPortfolioHandler) RegisterPayment(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	var req dto.RegisterLivestockCreditPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the body must be a JSON payment (invoiceId, amount, date, bank, reference)")
		return
	}
	out, err := h.service.RegisterPayment(c.Request.Context(), id, model.LivestockCreditPaymentRequest{
		InvoiceID: req.InvoiceID, Amount: req.Amount, Date: req.Date, Bank: req.Bank, Reference: req.Reference,
	}, actorOf(c))
	if err != nil {
		portfolioError(c, err)
		return
	}
	status := http.StatusCreated
	if out.Replayed || out.Payment.Status != model.PaymentRegistered {
		status = http.StatusOK
	}
	c.JSON(status, dto.LivestockCreditPaymentResult{
		Payment:   dto.ToLivestockCreditPaymentResponse(*out.Payment),
		Replayed:  out.Replayed,
		Portfolio: toPortfolioResponse(out.Portfolio),
	})
}

// SetBlocked godoc
// @Summary Block or unblock a rancher's purchases on the quota
// @Description Sends only credit.blocked to Odoo (the other credit fields belong to CuppoGan's service) and keeps the decision, its mandatory reason and who took it in Prestia's history. Odoo's own block for overdue installments is independent. Advisors and administrators.
// @Tags Livestock credit portfolio
// @Accept json
// @Produce json
// @Param id path string true "Credit line UUID"
// @Param request body dto.SetLivestockCreditBlockRequest true "Block or unblock, with the reason"
// @Success 200 {object} dto.LivestockCreditPortfolioResponse
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Failure 409 {object} dto.PortfolioErrorResponse
// @Failure 422 {object} dto.PortfolioErrorResponse
// @Failure 503 {object} dto.PortfolioErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/block [post]
func (h *LivestockCreditPortfolioHandler) SetBlocked(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	var req dto.SetLivestockCreditBlockRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Blocked == nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the body must be {\"blocked\": true|false, \"reason\": \"…\"}")
		return
	}
	p, err := h.service.SetBlocked(c.Request.Context(), id, *req.Blocked, req.Reason, actorOf(c))
	if err != nil {
		portfolioError(c, err)
		return
	}
	c.JSON(http.StatusOK, toPortfolioResponse(p))
}

// ListBanks godoc
// @Summary TVGAN banks Odoo accepts payments into
// @Description The options of the payment form's bank field, read from Odoo (cached for a few minutes). Advisors and administrators.
// @Tags Livestock credit portfolio
// @Produce json
// @Success 200 {object} dto.OdooBankList
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 503 {object} dto.PortfolioErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-portfolio/banks [get]
func (h *LivestockCreditPortfolioHandler) ListBanks(c *gin.Context) {
	banks, err := h.service.Banks(c.Request.Context())
	if err != nil {
		portfolioError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToOdooBankList(banks))
}

func toPortfolioResponse(p *service.LivestockCreditPortfolio) dto.LivestockCreditPortfolioResponse {
	return dto.ToLivestockCreditPortfolioResponse(p.Enabled, p.Line, p.Invoices, p.Payments, p.BlockEvents, p.Suggested)
}

func actorOf(c *gin.Context) service.Actor {
	var a service.Actor
	if v, ok := c.Get("userID"); ok {
		if id, isUUID := v.(uuid.UUID); isUUID {
			a.UserID = &id
		}
	}
	if v, ok := c.Get("email"); ok {
		a.Email, _ = v.(string)
	}
	return a
}

func portfolioError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrCreditLineNotFound) {
		codedError(c, http.StatusNotFound, "credit_line_not_found", "no credit line with that id")
		return
	}
	r, ok := service.AsPortfolioRejection(err)
	if !ok {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "the portfolio operation failed"})
		return
	}
	status := http.StatusUnprocessableEntity
	switch r.Kind {
	case model.PortfolioConflict:
		status = http.StatusConflict
	case model.PortfolioUnavailable, model.PortfolioDisabled:
		status = http.StatusServiceUnavailable
		if r.Kind == model.PortfolioUnavailable {
			c.Header("Retry-After", strconv.Itoa(60))
		}
	}
	c.JSON(status, dto.PortfolioErrorResponse{Code: r.Code, Error: r.Message, OdooCode: r.OdooCode, Retryable: r.Retryable()})
}
