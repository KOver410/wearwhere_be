package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type BrandFulfillmentHandler struct{ svc *service.FulfillmentService }

func NewBrandFulfillmentHandler(s *service.FulfillmentService) *BrandFulfillmentHandler {
	return &BrandFulfillmentHandler{svc: s}
}

func (h *BrandFulfillmentHandler) brandID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(brandmw.CtxBrandID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func (h *BrandFulfillmentHandler) List(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var statuses []domain.SubOrderStatus
	if s := c.Query("status"); s != "" {
		statuses = append(statuses, domain.SubOrderStatus(s))
	}
	resp, err := h.svc.List(c.Request.Context(), bid, statuses, page, pageSize)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BrandFulfillmentHandler) Detail(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	resp, err := h.svc.Detail(c.Request.Context(), bid, id)
	if err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BrandFulfillmentHandler) Confirm(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	if err := h.svc.Confirm(c.Request.Context(), bid, id); err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

func (h *BrandFulfillmentHandler) Ship(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	var req domain.ShipReq
	_ = c.ShouldBindJSON(&req) // body optional
	if err := h.svc.Ship(c.Request.Context(), bid, id, req.Carrier); err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "shipped"})
}

func writeFulfilErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSubOrderNotFound):
		httpx.Error(c, http.StatusNotFound, "SUB_ORDER_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrNotBrandOwner):
		httpx.Error(c, http.StatusForbidden, "NOT_OWNER", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		httpx.Error(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, domain.ErrCarrierUnavailable):
		httpx.Error(c, http.StatusConflict, "CARRIER_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrAddressIncomplete):
		httpx.Error(c, http.StatusConflict, "ADDRESS_INCOMPLETE", err.Error())
	case errors.Is(err, domain.ErrShipmentCreateFailed):
		httpx.Error(c, http.StatusBadGateway, "SHIPMENT_FAILED", err.Error())
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
