package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/handler"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
)

type fakeReadRepo struct {
	gotFilter domain.ListUsersFilter
	rows      []domain.AdminUserRow
	total     int
}

func (f *fakeReadRepo) ListUsers(_ context.Context, flt domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	f.gotFilter = flt
	return f.rows, f.total, nil
}

func setup(fake *fakeReadRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(service.New(fake))
	handler.MountAdmin(r.Group("/admin"), h)
	return r
}

func TestList_Returns200WithData(t *testing.T) {
	fake := &fakeReadRepo{rows: []domain.AdminUserRow{{ID: uuid.New(), Name: "Alice"}}, total: 1}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body domain.AdminUserListResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 1, body.Total)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "Alice", body.Data[0].Name)
}

func TestList_ParsesQueryParamsIntoFilter(t *testing.T) {
	fake := &fakeReadRepo{}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/admin/users?q=bob&sort=last_login_at&order=asc&page=2&page_size=500", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// Service normalizes before the repo sees it: page_size capped to 100.
	assert.Equal(t, "bob", fake.gotFilter.Q)
	assert.Equal(t, domain.SortLastLogin, fake.gotFilter.Sort)
	assert.Equal(t, domain.OrderAsc, fake.gotFilter.Order)
	assert.Equal(t, 2, fake.gotFilter.Page)
	assert.Equal(t, 100, fake.gotFilter.PageSize)
}

func TestList_EmptyDefaults(t *testing.T) {
	fake := &fakeReadRepo{}
	r := setup(fake)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, domain.SortCreatedAt, fake.gotFilter.Sort)
	assert.Equal(t, domain.OrderDesc, fake.gotFilter.Order)
	assert.Equal(t, 1, fake.gotFilter.Page)
	assert.Equal(t, 20, fake.gotFilter.PageSize)
}
