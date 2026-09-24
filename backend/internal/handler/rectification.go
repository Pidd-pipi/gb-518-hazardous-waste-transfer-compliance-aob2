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

type RectificationHandler struct {
	service service.RectificationService
}

func NewRectificationHandler(s service.RectificationService) *RectificationHandler {
	return &RectificationHandler{service: s}
}

func (h *RectificationHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/checks/:id/rectification")
	resource.GET("", middleware.RequireMinimumRole(model.RoleViewer), h.get)
	resource.POST("/open", middleware.RequireMinimumRole(model.RoleReviewer), h.open)
	resource.POST("/submit", middleware.RequireMinimumRole(model.RoleOperator), h.submit)
	resource.POST("/review", middleware.RequireMinimumRole(model.RoleReviewer), h.review)
}

func (h *RectificationHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.service.GetByCheckID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}

func (h *RectificationHandler) open(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.StartRectificationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service.Open(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, view)
}

func (h *RectificationHandler) submit(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.SubmitRectificationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service.Submit(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}

func (h *RectificationHandler) review(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.RecheckRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service.Review(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}
