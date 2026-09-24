package handler

import (
	"net/http"
	"strconv"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/dto"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/middleware"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/model"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/service"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type ComplianceCheckHandler struct {
	service service.ComplianceCheckService
}

func NewComplianceCheckHandler(s service.ComplianceCheckService) *ComplianceCheckHandler {
	return &ComplianceCheckHandler{service: s}
}

func (h *ComplianceCheckHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/checks")
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("", middleware.RequireMinimumRole(model.RoleOperator), h.create)
	resource.PUT("/:id", middleware.RequireMinimumRole(model.RoleOperator), h.update)
	resource.POST("/:id/transition", middleware.RequireMinimumRole(model.RoleReviewer), h.transition)
	resource.GET("/:id/remediation", h.remediation)
	resource.POST("/:id/remediation/rounds/:roundId/submit", middleware.RequireMinimumRole(model.RoleOperator), h.submitRemediation)
	resource.POST("/:id/remediation/rounds/:roundId/review", middleware.RequireMinimumRole(model.RoleReviewer), h.reviewRemediation)
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
	var input dto.DecideComplianceCheck
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

func (h *ComplianceCheckHandler) remediation(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.service.GetRemediation(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, detail)
}

func (h *ComplianceCheckHandler) submitRemediation(c *gin.Context) {
	id, roundID, ok := parseCheckAndRoundIDs(c)
	if !ok {
		return
	}
	var input dto.SubmitRemediationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.SubmitRemediation(c.Request.Context(), id, roundID, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ComplianceCheckHandler) reviewRemediation(c *gin.Context) {
	id, roundID, ok := parseCheckAndRoundIDs(c)
	if !ok {
		return
	}
	var input dto.ReviewRemediationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.ReviewRemediation(c.Request.Context(), id, roundID, input, actorFromContext(c), requestIDFromContext(c))
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

func parseCheckAndRoundIDs(c *gin.Context) (uint, uint, bool) {
	id, ok := parseID(c)
	if !ok {
		return 0, 0, false
	}
	roundID, err := strconv.ParseUint(c.Param("roundId"), 10, 64)
	if err != nil || roundID == 0 {
		util.Fail(c, http.StatusBadRequest, "invalid_request", "round id is required")
		return 0, 0, false
	}
	return id, uint(roundID), true
}
