//go:build integration

package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestAddressPG_CreateThenList(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)

	repo := NewAddressPG(tx)
	addr, err := repo.Create(context.Background(), sb.ID, &domain.CreateAddressRequest{
		Label: "HQ", AddressLine: "12 Phố Huế",
		Ward: "Ngô Thì Nhậm", District: "Hai Bà Trưng", City: "Hà Nội",
		IsPrimary: true,
	})
	require.NoError(t, err)
	require.True(t, addr.IsPrimary)
	require.Equal(t, "VN", addr.Country) // default

	items, err := repo.List(context.Background(), sb.ID, true)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestAddressPG_OnlyOnePrimary(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	repo := NewAddressPG(tx)
	ctx := context.Background()

	a1, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
		Label: "First", AddressLine: "x", Ward: "x", District: "x", City: "x",
		IsPrimary: true,
	})
	require.NoError(t, err)
	require.True(t, a1.IsPrimary)

	a2, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
		Label: "Second", AddressLine: "y", Ward: "y", District: "y", City: "y",
		IsPrimary: true,
	})
	require.NoError(t, err)
	require.True(t, a2.IsPrimary)

	// a1 must have been demoted by the create
	fetched, err := repo.FindByID(ctx, a1.ID, sb.ID)
	require.NoError(t, err)
	require.False(t, fetched.IsPrimary)
}

func TestAddressPG_IDORProtected(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sbA := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sbB := testfixtures.SeedBrand(t, tx, uuid.Nil)
	repo := NewAddressPG(tx)
	ctx := context.Background()

	addr, err := repo.Create(ctx, sbA.ID, &domain.CreateAddressRequest{
		Label: "x", AddressLine: "x", Ward: "x", District: "x", City: "x",
	})
	require.NoError(t, err)

	// Brand B must not see brand A's address.
	_, err = repo.FindByID(ctx, addr.ID, sbB.ID)
	require.ErrorIs(t, err, ErrNotFound)

	err = repo.SoftDelete(ctx, addr.ID, sbB.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestAddressPG_PublicOnlyFilter(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	repo := NewAddressPG(tx)
	ctx := context.Background()

	pubFalse := false
	_, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
		Label: "Public", AddressLine: "x", Ward: "x", District: "x", City: "x",
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
		Label: "Private", AddressLine: "y", Ward: "y", District: "y", City: "y",
		IsPublic: &pubFalse,
	})
	require.NoError(t, err)

	publicOnly, err := repo.List(ctx, sb.ID, false)
	require.NoError(t, err)
	require.Len(t, publicOnly, 1)
	require.Equal(t, "Public", publicOnly[0].Label)

	all, err := repo.List(ctx, sb.ID, true)
	require.NoError(t, err)
	require.Len(t, all, 2)
}
