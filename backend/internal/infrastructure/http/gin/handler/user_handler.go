package handler

import (
	"net/http"
	"strconv"

	"github.com/diogenes-moreira/creditos/backend/internal/application/dto"
	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// ListUsers godoc
// @Summary List users
// @Description Lists every user, optionally filtered by role, activation state or a search over email and phone
// @Tags Admin Users
// @Produce json
// @Param role query string false "admin, client or vendor"
// @Param active query boolean false "Filter by activation state"
// @Param search query string false "Match email or phone"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param offset query int false "Rows to skip"
// @Success 200 {object} dto.UserListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	filter := port.UserFilter{
		Role:   model.UserRole(c.Query("role")),
		Search: c.Query("search"),
	}
	if raw := c.Query("active"); raw != "" {
		active, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "active must be true or false"})
			return
		}
		filter.Active = &active
	}
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			filter.Limit = n
		}
	}
	if raw := c.Query("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			filter.Offset = n
		}
	}

	users, total, err := h.userService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToUserListResponse(users, total))
}

// CreateUser godoc
// @Summary Create a user
// @Description Registers a user with a role directly, which is how an administrator is added
// @Tags Admin Users
// @Accept json
// @Produce json
// @Param request body dto.CreateUserRequest true "User data"
// @Success 201 {object} dto.UserResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	actorID := c.MustGet("userID").(uuid.UUID)

	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.userService.Create(c.Request.Context(), actorID, req.Email, req.Phone, model.UserRole(req.Role))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ToUserResponse(user))
}

// ChangeUserRole godoc
// @Summary Change a user's role
// @Description Moves a user to another role. An administrator cannot change their own role.
// @Tags Admin Users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body dto.ChangeRoleRequest true "New role"
// @Success 200 {object} dto.UserResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/users/{id}/role [put]
func (h *UserHandler) ChangeUserRole(c *gin.Context) {
	actorID := c.MustGet("userID").(uuid.UUID)
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid user id"})
		return
	}

	var req dto.ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.userService.ChangeRole(c.Request.Context(), actorID, targetID, model.UserRole(req.Role))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToUserResponse(user))
}

// SetUserActive godoc
// @Summary Activate or deactivate a user
// @Description Revokes or restores a user's access. Accounts are never deleted, so the financial trail stays intact.
// @Tags Admin Users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body dto.SetActiveRequest true "Activation state"
// @Success 200 {object} dto.UserResponse
// @Failure 400 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /admin/users/{id}/active [put]
func (h *UserHandler) SetUserActive(c *gin.Context) {
	actorID := c.MustGet("userID").(uuid.UUID)
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid user id"})
		return
	}

	var req dto.SetActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.userService.SetActive(c.Request.Context(), actorID, targetID, *req.Active)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ToUserResponse(user))
}
