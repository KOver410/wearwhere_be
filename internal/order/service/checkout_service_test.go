package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cartdomain "github.com/wearwhere/wearwhere_be/internal/cart/domain"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	customeraddrdomain "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeCartRepo satisfies the full cartrepo.CartRepo interface.
type fakeCartRepo struct {
	items []*cartdomain.CartItemView
}

var _ cartrepo.CartRepo = (*fakeCartRepo)(nil)

func (f *fakeCartRepo) ListView(ctx context.Context, userID uuid.UUID) ([]*cartdomain.CartItemView, error) {
	return f.items, nil
}
func (f *fakeCartRepo) UpsertAdd(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*cartdomain.CartItem, error) {
	panic("unused in checkout tests")
}
func (f *fakeCartRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*cartdomain.CartItem, error) {
	panic("unused in checkout tests")
}
func (f *fakeCartRepo) FindByVariant(_ context.Context, _, _ uuid.UUID) (*cartdomain.CartItem, error) {
	panic("unused in checkout tests")
}
func (f *fakeCartRepo) UpdateQty(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*cartdomain.CartItem, error) {
	panic("unused in checkout tests")
}
func (f *fakeCartRepo) Delete(_ context.Context, _, _ uuid.UUID) error {
	panic("unused in checkout tests")
}
func (f *fakeCartRepo) Clear(_ context.Context, _ uuid.UUID) error {
	panic("unused in checkout tests")
}

// fakeAddrRepo satisfies the full customeraddrrepo.AddressRepo interface.
type fakeAddrRepo struct {
	addr    *customeraddrdomain.CustomerAddress
	findErr error
}

var _ customeraddrrepo.AddressRepo = (*fakeAddrRepo)(nil)

func (f *fakeAddrRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*customeraddrdomain.CustomerAddress, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.addr, nil
}
func (f *fakeAddrRepo) List(_ context.Context, _ uuid.UUID) ([]*customeraddrdomain.CustomerAddress, error) {
	panic("unused in checkout tests")
}
func (f *fakeAddrRepo) Create(_ context.Context, _ uuid.UUID, _ *customeraddrdomain.CreateAddressRequest) (*customeraddrdomain.CustomerAddress, error) {
	panic("unused in checkout tests")
}
func (f *fakeAddrRepo) Update(_ context.Context, _, _ uuid.UUID, _ *customeraddrdomain.UpdateAddressRequest) (*customeraddrdomain.CustomerAddress, error) {
	panic("unused in checkout tests")
}
func (f *fakeAddrRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error {
	panic("unused in checkout tests")
}

// stubProvider satisfies provider.ShippingProvider.
type stubProvider struct {
	opts []shippingdomain.ShippingOption
	err  error
}

var _ provider.ShippingProvider = (stubProvider{})

func (s stubProvider) Quote(_ context.Context, _ provider.CalcReq) ([]shippingdomain.ShippingOption, error) {
	return s.opts, s.err
}

// fakeShipping wraps stubProvider for tests that only care about a flat fee.
func newFakeShipping(fee int64) stubProvider {
	return stubProvider{opts: []shippingdomain.ShippingOption{
		{Carrier: "flat", CarrierName: "Flat Rate", AmountVND: fee, ETA: ""},
	}}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func makeAddr(userID uuid.UUID) *customeraddrdomain.CustomerAddress {
	city := "79"
	district := "760"
	ward := "26734"
	return &customeraddrdomain.CustomerAddress{
		ID:             uuid.New(),
		UserID:         userID,
		RecipientName:  "Nguyen Van A",
		RecipientPhone: "0901234567",
		AddressLine:    "123 Le Loi",
		Ward:           "Ben Nghe",
		District:       "Quan 1",
		City:           "Ho Chi Minh",
		CityCode:       &city,
		DistrictCode:   &district,
		WardCode:       &ward,
	}
}

func makeAddrNoCodes(userID uuid.UUID) *customeraddrdomain.CustomerAddress {
	return &customeraddrdomain.CustomerAddress{
		ID:             uuid.New(),
		UserID:         userID,
		RecipientName:  "Nguyen Van B",
		RecipientPhone: "0909999999",
		AddressLine:    "456 Nguyen Hue",
		Ward:           "Ben Thanh",
		District:       "Quan 1",
		City:           "Ho Chi Minh",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPreview_EmptyCart(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	addr := makeAddr(userID)
	addr.ID = addrID

	svc := NewCheckoutService(
		&fakeCartRepo{items: []*cartdomain.CartItemView{}},
		&fakeAddrRepo{addr: addr},
		newFakeShipping(30000),
	)

	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.True(t, resp.CartEmpty)
	assert.Empty(t, resp.SubOrders)
	assert.False(t, resp.MeetsMinOrder)
	assert.Equal(t, domain.MinOrderValueVND, resp.MinOrderValueVND)
	assert.NotNil(t, resp.Address)
	assert.Equal(t, "Nguyen Van A", resp.Address.Recipient)
}

func TestPreview_AddressNotOwned_Returns404(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()

	svc := NewCheckoutService(
		&fakeCartRepo{items: []*cartdomain.CartItemView{}},
		// FindByID returns ErrNotFound (simulates wrong owner or missing addr)
		&fakeAddrRepo{findErr: customeraddrrepo.ErrNotFound},
		newFakeShipping(30000),
	)

	_, err := svc.Preview(context.Background(), userID, addrID)
	assert.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestPreview_GroupsByBrand_AndComputesTotals(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	addr := makeAddr(userID)
	addr.ID = addrID

	brandA := uuid.New()
	brandB := uuid.New()
	varA1 := uuid.New()
	varB1 := uuid.New()

	unavailReason := "variant_deleted"
	items := []*cartdomain.CartItemView{
		{
			VariantID: varA1, ProductID: uuid.New(),
			ProductName: "Shirt A", SKU: "SA-001", Color: "Red", Size: "M",
			Qty: 2, CurrentPrice: 100000,
			BrandID: brandA, BrandSlug: "brand-a", BrandName: "Brand A",
			StockQty: 5, Unavailable: false,
			PrimaryImageURL: strPtr("https://img/a.jpg"),
		},
		{
			VariantID: varB1, ProductID: uuid.New(),
			ProductName: "Pants B", SKU: "PB-001", Color: "", Size: "32",
			Qty: 1, CurrentPrice: 200000,
			BrandID: brandB, BrandSlug: "brand-b", BrandName: "Brand B",
			StockQty: 10, Unavailable: false,
			PrimaryImageURL: nil,
		},
		// unavailable item — should be skipped and add a warning
		{
			VariantID: uuid.New(), ProductID: uuid.New(),
			ProductName: "Gone", SKU: "GONE-1",
			Qty: 1, CurrentPrice: 50000,
			BrandID: brandA, BrandSlug: "brand-a", BrandName: "Brand A",
			StockQty: 0, Unavailable: true, UnavailableReason: &unavailReason,
		},
	}

	const shippingPerBrand int64 = 30000

	svc := NewCheckoutService(
		&fakeCartRepo{items: items},
		&fakeAddrRepo{addr: addr},
		newFakeShipping(shippingPerBrand),
	)

	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.False(t, resp.CartEmpty)
	assert.Len(t, resp.SubOrders, 2)

	// subtotals: brandA = 2*100000 = 200000, brandB = 1*200000 = 200000
	assert.Equal(t, int64(400000), resp.SubtotalVND)
	// shipping: 2 brands × 30000 = 60000
	assert.Equal(t, int64(60000), resp.ShippingTotalVND)
	assert.Equal(t, int64(460000), resp.GrandTotalVND)
	assert.True(t, resp.MeetsMinOrder)

	// one warning for the unavailable variant
	assert.Len(t, resp.Warnings, 1)
	assert.Contains(t, resp.Warnings[0], "unavailable")

	// check variant label helpers
	var shirtSubOrder *domain.CheckoutPreviewSubOrder
	for i := range resp.SubOrders {
		for _, it := range resp.SubOrders[i].Items {
			if it.VariantID == varA1 {
				shirtSubOrder = &resp.SubOrders[i]
			}
		}
	}
	require.NotNil(t, shirtSubOrder)
	assert.Equal(t, "Red / M", shirtSubOrder.Items[0].VariantLabel)

	var pantsSubOrder *domain.CheckoutPreviewSubOrder
	for i := range resp.SubOrders {
		for _, it := range resp.SubOrders[i].Items {
			if it.VariantID == varB1 {
				pantsSubOrder = &resp.SubOrders[i]
			}
		}
	}
	require.NotNil(t, pantsSubOrder)
	assert.Equal(t, "32", pantsSubOrder.Items[0].VariantLabel)
}

func TestPreview_BelowMinOrder(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	addr := makeAddr(userID)
	addr.ID = addrID

	// price 10000 × qty 1 = 10000, below MinOrderValueVND (50000)
	items := []*cartdomain.CartItemView{
		{
			VariantID: uuid.New(), ProductID: uuid.New(),
			ProductName: "Cheap Tee", SKU: "CT-001", Color: "Blue", Size: "S",
			Qty: 1, CurrentPrice: 10000,
			BrandID: uuid.New(), BrandSlug: "brand-c", BrandName: "Brand C",
			StockQty: 5, Unavailable: false,
		},
	}

	svc := NewCheckoutService(
		&fakeCartRepo{items: items},
		&fakeAddrRepo{addr: addr},
		newFakeShipping(20000),
	)

	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.False(t, resp.CartEmpty)
	assert.False(t, resp.MeetsMinOrder)
	assert.Equal(t, int64(10000), resp.SubtotalVND)
	assert.Equal(t, domain.MinOrderValueVND, resp.MinOrderValueVND)
}

func TestPreview_ReturnsCarrierOptions(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	addr := makeAddr(userID) // has CityCode/DistrictCode/WardCode set
	addr.ID = addrID

	items := []*cartdomain.CartItemView{
		{
			VariantID: uuid.New(), ProductID: uuid.New(),
			ProductName: "Shirt X", SKU: "SX-001", Color: "Blue", Size: "M",
			Qty: 1, CurrentPrice: 150000,
			BrandID: uuid.New(), BrandSlug: "brand-x", BrandName: "Brand X",
			StockQty: 5, Unavailable: false,
		},
	}

	sp := stubProvider{opts: []shippingdomain.ShippingOption{
		{Carrier: "ghnv3", CarrierName: "GHN", Service: "standard", AmountVND: 25000, ETA: "2 ngày"},
		{Carrier: "ghtk", CarrierName: "GHTK", Service: "standard", AmountVND: 20000, ETA: "3 ngày"},
	}}

	svc := NewCheckoutService(
		&fakeCartRepo{items: items},
		&fakeAddrRepo{addr: addr},
		sp,
	)

	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.False(t, resp.AddressIncomplete, "should not be incomplete")
	require.Len(t, resp.SubOrders, 1)
	assert.Len(t, resp.SubOrders[0].ShippingOptions, 2, "want 2 options")
	// cheapest is 20000 (ghtk), so shipping fee and grand total should use that
	assert.Equal(t, int64(20000), resp.SubOrders[0].ShippingFeeVND)
	assert.Equal(t, int64(20000), resp.ShippingTotalVND)
}

func TestPreview_AddressIncompleteWhenNoCodes(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	addr := makeAddrNoCodes(userID) // no CityCode/DistrictCode/WardCode
	addr.ID = addrID

	items := []*cartdomain.CartItemView{
		{
			VariantID: uuid.New(), ProductID: uuid.New(),
			ProductName: "Pants Y", SKU: "PY-001", Color: "Black", Size: "32",
			Qty: 1, CurrentPrice: 200000,
			BrandID: uuid.New(), BrandSlug: "brand-y", BrandName: "Brand Y",
			StockQty: 3, Unavailable: false,
		},
	}

	svc := NewCheckoutService(
		&fakeCartRepo{items: items},
		&fakeAddrRepo{addr: addr},
		stubProvider{}, // provider should NOT be called when address is incomplete
	)

	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.True(t, resp.AddressIncomplete, "want AddressIncomplete=true")
	require.Len(t, resp.SubOrders, 1)
	assert.Empty(t, resp.SubOrders[0].ShippingOptions)
	assert.Equal(t, int64(0), resp.SubOrders[0].ShippingFeeVND)
}
