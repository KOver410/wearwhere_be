package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
)

const (
	ctxUserID = "auth.user_id"
	ctxRole   = "auth.role"
	ctxEmail  = "auth.email"
)

func setAuthCtx(c *gin.Context, userID uuid.UUID, role, email string) {
	c.Set(ctxUserID, userID)
	c.Set(ctxRole, role)
	c.Set(ctxEmail, email)
}

func UserID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(ctxUserID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func Role(c *gin.Context) domain.Role {
	v, _ := c.Get(ctxRole)
	if s, ok := v.(string); ok {
		return domain.Role(s)
	}
	return ""
}
