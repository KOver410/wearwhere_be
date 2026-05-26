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

func TestOrderItemPG_CreateAndList(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	ctx := context.Background()

	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 50000, 10)

	orepo := orderrepo.NewOrderPG(tx)
	srepo := orderrepo.NewSubOrderPG(tx)
	irepo := orderrepo.NewOrderItemPG(tx)

	o := seedOrderVal(user.ID, "WW-20260524-OIPG")
	require.NoError(t, orepo.Create(ctx, tx, o))

	so := &domain.SubOrder{
		OrderID:        o.ID,
		BrandID:        brand.ID,
		SubtotalVND:    100000,
		ShippingFeeVND: 0,
		TotalVND:       100000,
		Status:         domain.SubOrderStatusPending,
	}
	require.NoError(t, srepo.Create(ctx, tx, so))

	img := "http://img/1.jpg"
	it := &domain.OrderItem{
		SubOrderID:   so.ID,
		VariantID:    variantID,
		ProductID:    prod.ID,
		ProductName:  "Tee",
		VariantLabel: "Black / M",
		ImageURL:     &img,
		Qty:          2,
		UnitPriceVND: 50000,
		LineTotalVND: 100000,
	}
	require.NoError(t, irepo.Create(ctx, tx, it))
	require.NotEqual(t, uuid.Nil, it.ID)

	list, err := irepo.ListBySubOrderID(ctx, so.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "Tee", list[0].ProductName)

	byOrder, err := irepo.ListByOrderID(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, byOrder, 1)
	require.Equal(t, it.ID, byOrder[0].ID)
}
