package handler

import (
	"net/http"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/dto"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/middleware"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/model"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/service"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type ComplianceCheckHandler struct {
	service        service.ComplianceCheckService
	rectifications service.RectificationService
}

func NewComplianceCheckHandler(s service.ComplianceCheckService, rectifications service.RectificationService) *ComplianceCheckHandler {
	return &ComplianceCheckHandler{service: s, rectifications: rectifications}
}

func (h *ComplianceCheckHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/checks")
	resource.GET("", h.list)
	resource.GET("/rectification-summaries", middleware.RequireMinimumRole(model.RoleViewer), h.rectificationSummaries)
	resource.GET("/:id", h.get)
	resource.POST("", middleware.RequireMinimumRole(model.RoleOperator), h.create)
	resource.PUT("/:id", middleware.RequireMinimumRole(model.RoleOperator), h.update)
	resource.POST("/:id/transition", middleware.RequireMinimumRole(model.RoleReviewer), h.transition)
	resource.DELETE("/:id", middleware.RequireRoles(model.RoleAdmin), h.remove)
}

func (h *ComplianceCheckHandler) list(c *gin.Context) {
	query := bindPage(c)
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

// rectificationSummaries returns progress/deadline/latest-note rows for the
// checks visible on the current list page so the table can render remediation
// status without one request per row.
func (h *ComplianceCheckHandler) rectificationSummaries(c *gin.Context) {
	var input struct {
		IDs []uint `form:"ids" binding:"required,min=1,max=100"`
	}
	if err := c.ShouldBindQuery(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	summaries, err := h.rectifications.Summaries(c.Request.Context(), input.IDs)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, summaries)
}

func (h *ComplianceCheckHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ComplianceCheckHandler) create(c *gin.Context) {
	var input dto.CreateComplianceCheck
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Create(c.Request.Context(), input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, item)
}

func (h *ComplianceCheckHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.UpdateComplianceCheck
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ComplianceCheckHandler) transition(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.TransitionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Transition(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ComplianceCheckHandler) remove(c *gin.Context) {
	if roleFromContext(c) != "admin" {
		util.Fail(c, http.StatusForbidden, "forbidden", "admin role is required")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, actorFromContext(c), requestIDFromContext(c)); err != nil {
		handleError(c, err)
		return
	}
	util.NoContent(c)
}
