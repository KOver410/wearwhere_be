package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

func TestNormalized_Defaults(t *testing.T) {
	got := domain.ListUsersFilter{}.Normalized()
	assert.Equal(t, domain.SortCreatedAt, got.Sort)
	assert.Equal(t, domain.OrderDesc, got.Order)
	assert.Equal(t, 1, got.Page)
	assert.Equal(t, 20, got.PageSize)
	assert.Equal(t, "", got.Q)
}

func TestNormalized_ClampsAndFallbacks(t *testing.T) {
	got := domain.ListUsersFilter{
		Q: "  alice  ", Sort: "bogus", Order: "sideways", Page: -3, PageSize: 500,
	}.Normalized()
	assert.Equal(t, "alice", got.Q)             // trimmed
	assert.Equal(t, domain.SortCreatedAt, got.Sort)  // unknown -> default
	assert.Equal(t, domain.OrderDesc, got.Order)     // unknown -> default
	assert.Equal(t, 1, got.Page)                // <1 -> 1
	assert.Equal(t, 100, got.PageSize)          // >100 -> 100
}

func TestNormalized_KeepsValidValues(t *testing.T) {
	got := domain.ListUsersFilter{
		Sort: domain.SortLastLogin, Order: domain.OrderAsc, Page: 3, PageSize: 50,
	}.Normalized()
	assert.Equal(t, domain.SortLastLogin, got.Sort)
	assert.Equal(t, domain.OrderAsc, got.Order)
	assert.Equal(t, 3, got.Page)
	assert.Equal(t, 50, got.PageSize)
}

func TestNormalized_ZeroPageSizeDefaults(t *testing.T) {
	assert.Equal(t, 20, domain.ListUsersFilter{PageSize: 0}.Normalized().PageSize)
}

func TestToResp_MapsAndDerivesVerifiedFlags(t *testing.T) {
	now := time.Now()
	email := "a@b.com"
	id := uuid.New()
	row := domain.AdminUserRow{
		ID: id, Email: &email, Phone: nil, Name: "Alice",
		Role: "customer", Status: "active",
		EmailVerifiedAt: &now, PhoneVerifiedAt: nil,
		CreatedAt: now,
	}
	resp := domain.ToResp(row)
	assert.Equal(t, id.String(), resp.ID)
	assert.Equal(t, &email, resp.Email)
	assert.Equal(t, "Alice", resp.Name)
	assert.True(t, resp.EmailVerified)
	assert.False(t, resp.PhoneVerified)
}
