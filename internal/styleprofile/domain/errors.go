package domain

import (
	"net/http"
	"strings"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// ErrInvalidBudget is returned when budget_max < budget_min.
var ErrInvalidBudget = httpx.NewAppError(
	http.StatusBadRequest, "VALIDATION_FAILED", "budget_max must be >= budget_min",
)

// UnknownStyleTagsError carries the style tag IDs that do not exist so the
// handler can surface them in the Format-A error details.
type UnknownStyleTagsError struct{ IDs []string }

func (e *UnknownStyleTagsError) Error() string {
	return "unknown style tag ids: " + strings.Join(e.IDs, ",")
}
