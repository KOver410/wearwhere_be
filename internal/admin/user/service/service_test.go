package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
)

// fakeReadRepo records the filter it was called with and returns canned data.
type fakeReadRepo struct {
	gotFilter domain.ListUsersFilter
	rows      []domain.AdminUserRow
	total     int
	err       error
}

func (f *fakeReadRepo) ListUsers(_ context.Context, flt domain.ListUsersFilter) ([]domain.AdminUserRow, int, error) {
	f.gotFilter = flt
	return f.rows, f.total, f.err
}

func TestListUsers_NormalizesFilterBeforeRepo(t *testing.T) {
	fake := &fakeReadRepo{}
	svc := service.New(fake)
	_, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{PageSize: 999, Sort: "x"})
	require.NoError(t, err)
	assert.Equal(t, 100, fake.gotFilter.PageSize)        // clamped
	assert.Equal(t, domain.SortCreatedAt, fake.gotFilter.Sort) // fallback
	assert.Equal(t, 1, fake.gotFilter.Page)              // default
}

func TestListUsers_MapsRowsAndPagination(t *testing.T) {
	fake := &fakeReadRepo{
		rows:  []domain.AdminUserRow{{ID: uuid.New(), Name: "A"}, {ID: uuid.New(), Name: "B"}},
		total: 134,
	}
	svc := service.New(fake)
	resp, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "A", resp.Data[0].Name)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
	assert.Equal(t, 134, resp.Total)
	assert.Equal(t, 7, resp.TotalPages) // ceil(134/20)
}

func TestListUsers_EmptyResult(t *testing.T) {
	svc := service.New(&fakeReadRepo{rows: nil, total: 0})
	resp, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{})
	require.NoError(t, err)
	assert.NotNil(t, resp.Data) // non-nil empty slice -> serializes as []
	assert.Len(t, resp.Data, 0)
	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 0, resp.TotalPages)
}

func TestListUsers_RepoErrorPropagates(t *testing.T) {
	svc := service.New(&fakeReadRepo{err: errors.New("db down")})
	_, err := svc.ListUsers(context.Background(), domain.ListUsersFilter{})
	require.Error(t, err)
}
