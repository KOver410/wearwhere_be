//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestSubOrderPG_CreateAndList(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	ctx := context.Background()

	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)

	orepo := orderrepo.NewOrderPG(tx)
	srepo := orderrepo.NewSubOrderPG(tx)

	o := seedOrderVal(user.ID, "WW-20260524-SOSO")
	require.NoError(t, orepo.Create(ctx, tx, o))

	so := &domain.SubOrder{
		OrderID:        o.ID,
		BrandID:        brand.ID,
		SubtotalVND:    100000,
		ShippingFeeVND: 30000,
		TotalVND:       130000,
		Status:         domain.SubOrderStatusPending,
	}
	require.NoError(t, srepo.Create(ctx, tx, so))
	require.NotEqual(t, uuid.Nil, so.ID)

	list, err := srepo.ListByOrderID(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, brand.Slug, list[0].BrandSlug)
	require.Equal(t, brand.Name, list[0].BrandName)
}

func TestSubOrderPG_CancelAllByOrderID(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	ctx := context.Background()

	user := testfixtures.SeedCustomer(t, tx)
	brand1 := testfixtures.SeedBrand(t, tx, uuid.Nil)
	brand2 := testfixtures.SeedBrand(t, tx, uuid.Nil)

	orepo := orderrepo.NewOrderPG(tx)
	srepo := orderrepo.NewSubOrderPG(tx)

	o := seedOrderVal(user.ID, "WW-20260524-CSOS")
	require.NoError(t, orepo.Create(ctx, tx, o))

	for _, brandID := range []uuid.UUID{brand1.ID, brand2.ID} {
		require.NoError(t, srepo.Create(ctx, tx, &domain.SubOrder{
			OrderID:        o.ID,
			BrandID:        brandID,
			SubtotalVND:    100,
			ShippingFeeVND: 0,
			TotalVND:       100,
			Status:         domain.SubOrderStatusPending,
		}))
	}

	require.NoError(t, srepo.CancelAllByOrderID(ctx, tx, o.ID))

	list, err := srepo.ListByOrderID(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	for _, item := range list {
		require.Equal(t, domain.SubOrderStatusCancelled, item.Status)
		require.NotNil(t, item.CancelledAt)
	}
}
