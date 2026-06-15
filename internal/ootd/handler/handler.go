// Package handler exposes OOTD HTTP endpoints.
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
	"github.com/wearwhere/wearwhere_be/internal/ootd/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func parsePage(c *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	return
}

func (h *Handler) Create(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	files := form.File["photos"]
	caption := c.PostForm("caption")
	var productIDs []uuid.UUID
	for _, s := range c.PostFormArray("product_ids") {
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "INVALID_PRODUCT_ID", "invalid product_id: "+s)
			return
		}
		productIDs = append(productIDs, id)
	}
	post, err := h.svc.CreatePost(c.Request.Context(), h.userID(c), caption, files, productIDs)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"id": post.ID.String()})
}

func (h *Handler) Following(c *gin.Context) {
	page, limit := parsePage(c)
	views, total, err := h.svc.Following(c.Request.Context(), h.userID(c), page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	h.respondList(c, views, total, page, limit)
}

func (h *Handler) Feed(c *gin.Context) {
	page, limit := parsePage(c)
	views, total, err := h.svc.Feed(c.Request.Context(), h.userID(c), page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	h.respondList(c, views, total, page, limit)
}

func (h *Handler) ByUser(c *gin.Context) {
	uid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid user id")
		return
	}
	page, limit := parsePage(c)
	views, total, err := h.svc.ByUser(c.Request.Context(), h.userID(c), uid, page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	h.respondList(c, views, total, page, limit)
}

func (h *Handler) respondList(c *gin.Context, views []*domain.PostView, total, page, limit int) {
	items := make([]domain.PostResponse, 0, len(views))
	for _, v := range views {
		items = append(items, domain.ToPostResponse(v))
	}
	httpx.OK(c, gin.H{"items": items, "pagination": domain.NewPagination(page, limit, total)})
}

func (h *Handler) Detail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	v, err := h.svc.GetPost(c.Request.Context(), h.userID(c), id)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToPostResponse(v))
}

func (h *Handler) UpdateCaption(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	var req domain.UpdateCaptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := h.svc.UpdateCaption(c.Request.Context(), h.userID(c), id, req.Caption); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"updated": true})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	if err := h.svc.DeletePost(c.Request.Context(), h.userID(c), id); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *Handler) Like(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	if err := h.svc.Like(c.Request.Context(), h.userID(c), id); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"liked": true})
}

func (h *Handler) Unlike(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	if err := h.svc.Unlike(c.Request.Context(), h.userID(c), id); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"liked": false})
}

func (h *Handler) AddComment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	var req domain.AddCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	cm, err := h.svc.AddComment(c.Request.Context(), h.userID(c), id, req.Body)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"id": cm.ID.String()})
}

func (h *Handler) ListComments(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPostNotFound())
		return
	}
	page, limit := parsePage(c)
	list, total, err := h.svc.ListComments(c.Request.Context(), id, page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	items := make([]domain.CommentResponse, 0, len(list))
	for _, cm := range list {
		items = append(items, domain.ToCommentResponse(cm))
	}
	httpx.OK(c, gin.H{"items": items, "pagination": domain.NewPagination(page, limit, total)})
}

func (h *Handler) DeleteComment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrCommentNotFound())
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), h.userID(c), id); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
