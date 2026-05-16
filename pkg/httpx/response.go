package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Error(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{Error: ErrorPayload{Code: code, Message: msg}})
}

func ErrorWithDetails(c *gin.Context, status int, code, msg string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{Error: ErrorPayload{Code: code, Message: msg, Details: details}})
}

// AppError is a tagged error services return to handlers; handlers translate it
// to the appropriate HTTP status via ErrorFromApp.
type AppError struct {
	Code    string
	Message string
	Status  int
}

func (e *AppError) Error() string { return e.Message }

func NewAppError(status int, code, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

func ErrorFromApp(c *gin.Context, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		Error(c, ae.Status, ae.Code, ae.Message)
		return
	}
	Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
