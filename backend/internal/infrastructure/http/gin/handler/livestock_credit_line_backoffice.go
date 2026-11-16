package handler

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterBackOfficeRoutes mounts the read-only credit line routes for the
// back office (advisors and administrators) on a group that already
// authenticates and checks the role.
func (h *LivestockCreditLineHandler) RegisterBackOfficeRoutes(g gin.IRoutes) {
	g.GET("/livestock-credit-lines", h.ListCreditLines)
	g.POST("/livestock-credit-lines/search", h.SearchCreditLines)
	g.GET("/livestock-credit-lines/:id", h.GetCreditLine)
	g.GET("/livestock-credit-lines/:id/promissory-note", h.GetPromissoryNote)
}

// ListCreditLines godoc
// @Summary List livestock credit lines
// @Description Credit lines partners (CuppoGan) activated on a livestock quota, most recently activated first. Metadata only: the signed promissory note is downloaded separately. To filter by national ID use POST /admin/livestock-credit-lines/search, which keeps the ID out of the URL. Advisors and administrators.
// @Tags Livestock quota
// @Produce json
// @Param status query string false "Status filter (active)"
// @Param offset query int false "Offset"
// @Param limit query int false "Page size (default 20, at most 100)"
// @Success 200 {object} dto.LivestockCreditLinePage
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines [get]
func (h *LivestockCreditLineHandler) ListCreditLines(c *gin.Context) {
	var q dto.LivestockCreditLineListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "offset and limit must be integers")
		return
	}
	h.list(c, port.LivestockCreditLineFilter{
		Status: model.LivestockCreditLineStatus(q.Status), Offset: q.Offset, Limit: q.Limit,
	})
}

// SearchCreditLines godoc
// @Summary Search livestock credit lines
// @Description The credit line list filtered by national ID (exact match) and status, with the filter in the body so the national ID stays out of URLs and request logs. Advisors and administrators.
// @Tags Livestock quota
// @Accept json
// @Produce json
// @Param request body dto.LivestockCreditLineSearchRequest true "Filter and page"
// @Success 200 {object} dto.LivestockCreditLinePage
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/search [post]
func (h *LivestockCreditLineHandler) SearchCreditLines(c *gin.Context) {
	var req dto.LivestockCreditLineSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the body must be a JSON filter")
		return
	}
	h.list(c, port.LivestockCreditLineFilter{
		NationalID: strings.TrimSpace(req.NationalID), Status: model.LivestockCreditLineStatus(req.Status),
		Offset: req.Offset, Limit: req.Limit,
	})
}

func (h *LivestockCreditLineHandler) list(c *gin.Context, f port.LivestockCreditLineFilter) {
	lines, total, f, err := h.service.List(c.Request.Context(), f)
	if errors.Is(err, service.ErrInvalidCreditLineStatus) {
		codedError(c, http.StatusBadRequest, "invalid_status", err.Error())
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "the credit lines cannot be read"})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockCreditLinePage(lines, total, f.Offset, f.Limit))
}

// GetCreditLine godoc
// @Summary Get a livestock credit line
// @Description One credit line with its signature trace and a summary of the quota assessment it was granted on. Advisors and administrators.
// @Tags Livestock quota
// @Produce json
// @Param id path string true "Credit line UUID"
// @Success 200 {object} dto.LivestockCreditLineDetail
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id} [get]
func (h *LivestockCreditLineHandler) GetCreditLine(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	line, assessment, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, service.ErrCreditLineNotFound) {
		codedError(c, http.StatusNotFound, "credit_line_not_found", "no credit line with that id")
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "the credit line cannot be read"})
		return
	}
	c.JSON(http.StatusOK, dto.ToLivestockCreditLineDetail(line, assessment))
}

// GetPromissoryNote godoc
// @Summary Download a credit line's signed promissory note
// @Description Streams the signed PDF stored with the line, inline, named after the promissory note number. The bytes are hashed on every read: if they no longer match the SHA-256 registered with the line the PDF is not served (500, promissory_note_corrupted). Every access is recorded in the audit log. Advisors and administrators.
// @Tags Livestock quota
// @Produce application/pdf
// @Param id path string true "Credit line UUID"
// @Success 200 {file} file "The signed promissory note"
// @Failure 400 {object} dto.CodedErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.CodedErrorResponse
// @Failure 500 {object} dto.CodedErrorResponse
// @Security BearerAuth
// @Router /admin/livestock-credit-lines/{id}/promissory-note [get]
func (h *LivestockCreditLineHandler) GetPromissoryNote(c *gin.Context) {
	id, ok := creditLineID(c)
	if !ok {
		return
	}
	var viewer *uuid.UUID
	if v, exists := c.Get("userID"); exists {
		if uid, isUUID := v.(uuid.UUID); isUUID {
			viewer = &uid
		}
	}
	line, doc, err := h.service.PromissoryNote(c.Request.Context(), id, viewer)
	switch {
	case errors.Is(err, service.ErrCreditLineNotFound):
		codedError(c, http.StatusNotFound, "credit_line_not_found", "no credit line or promissory note with that id")
		return
	case errors.Is(err, service.ErrPromissoryNoteCorrupted):
		codedError(c, http.StatusInternalServerError, "promissory_note_corrupted", err.Error())
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "the promissory note cannot be read"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s.pdf"`, promissoryNoteFileName(line.PromissoryNoteNumber)))
	c.Header("Content-Length", strconv.FormatInt(int64(len(doc.Content)), 10))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, model.PromissoryNoteContentType, doc.Content)
}

func creditLineID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		codedError(c, http.StatusBadRequest, "invalid_request", "the credit line id must be a UUID")
		return uuid.Nil, false
	}
	return id, true
}

var unsafeFileNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// promissoryNoteFileName keeps the note number safe inside a header.
func promissoryNoteFileName(number string) string {
	name := unsafeFileNameChars.ReplaceAllString(number, "_")
	if name == "" {
		return "pagare"
	}
	return name
}
