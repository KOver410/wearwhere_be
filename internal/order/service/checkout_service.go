package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	cartdomain "github.com/wearwhere/wearwhere_be/internal/cart/domain"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

// CheckoutService is a read-only service that returns a preview of what the
// order would look like if placed now. No DB writes, no reservation.
type CheckoutService struct {
	cartRepo cartrepo.CartRepo
	addrRepo customeraddrrepo.AddressRepo
	shipping provider.ShippingProvider
}

func NewCheckoutService(
	c cartrepo.CartRepo,
	a customeraddrrepo.AddressRepo,
	s provider.ShippingProvider,
) *CheckoutService {
	return &CheckoutService{cartRepo: c, addrRepo: a, shipping: s}
}

// Preview returns the would-be order: items grouped by brand, shipping fee per
// brand, grand totals, and any stock/availability warnings.
func (s *CheckoutService) Preview(
	ctx context.Context,
	userID, addressID uuid.UUID,
) (*domain.CheckoutPreviewResp, error) {
	// FindByID is already scoped to userID — it returns ErrNotFound if addr
	// belongs to a different user.
	addr, err := s.addrRepo.FindByID(ctx, addressID, userID)
	if err != nil {
		return nil, domain.ErrAddressNotFound
	}

	shipAddr := domain.ShippingAddress{
		Recipient:    addr.RecipientName,
		Phone:        addr.RecipientPhone,
		Line1:        addr.AddressLine,
		Ward:         addr.Ward,
		District:     addr.District,
		City:         addr.City,
		CityCode:     addr.CityCode,
		DistrictCode: addr.DistrictCode,
		WardCode:     addr.WardCode,
	}

	addrIncomplete := addr.CityCode == nil || addr.DistrictCode == nil || addr.WardCode == nil

	items, err := s.cartRepo.ListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &domain.CheckoutPreviewResp{
			CartEmpty:         true,
			Address:           &shipAddr,
			SubOrders:         []domain.CheckoutPreviewSubOrder{},
			MinOrderValueVND:  domain.MinOrderValueVND,
			MeetsMinOrder:     false,
			Warnings:          []string{},
			AddressIncomplete: addrIncomplete,
		}, nil
	}

	type group struct {
		brand    domain.BrandRef
		items    []domain.CheckoutPreviewItem
		subtotal int64
	}
	grouped := map[uuid.UUID]*group{}
	// preserve insertion order for deterministic output
	brandOrder := []uuid.UUID{}
	warnings := []string{}
	var subtotalAll int64

	for _, ci := range items {
		if ci.Unavailable {
			reason := ""
			if ci.UnavailableReason != nil {
				reason = *ci.UnavailableReason
			}
			warnings = append(warnings, fmt.Sprintf(
				"variant %s unavailable (%s)", ci.VariantID, reason,
			))
			continue
		}
		if ci.StockQty < ci.Qty {
			warnings = append(warnings, fmt.Sprintf(
				"variant %s low stock (available %d, in cart %d)",
				ci.VariantID, ci.StockQty, ci.Qty,
			))
		}

		lineTotal := int64(float64(ci.Qty) * ci.CurrentPrice)
		grp, ok := grouped[ci.BrandID]
		if !ok {
			grp = &group{
				brand: domain.BrandRef{
					ID:   ci.BrandID,
					Slug: ci.BrandSlug,
					Name: ci.BrandName,
				},
			}
			grouped[ci.BrandID] = grp
			brandOrder = append(brandOrder, ci.BrandID)
		}
		grp.items = append(grp.items, domain.CheckoutPreviewItem{
			VariantID:    ci.VariantID,
			ProductID:    ci.ProductID,
			ProductName:  ci.ProductName,
			VariantLabel: variantLabel(ci),
			ImageURL:     ci.PrimaryImageURL,
			Qty:          ci.Qty,
			UnitPriceVND: int64(ci.CurrentPrice),
			LineTotalVND: lineTotal,
			AvailableQty: ci.StockQty,
		})
		grp.subtotal += lineTotal
		subtotalAll += lineTotal
	}

	subOrders := make([]domain.CheckoutPreviewSubOrder, 0, len(grouped))
	var shippingAll int64
	for _, bID := range brandOrder {
		g := grouped[bID]

		var options []domain.ShippingOptionResp
		var cheapest int64
		if !addrIncomplete {
			opts, err := s.shipping.Quote(ctx, provider.CalcReq{
				BrandID:    bID,
				ToAddress:  toShippingProviderAddr(shipAddr),
				ToCityCode: addr.CityCode,
				ToDistrict: addr.DistrictCode,
				CODVND:     0,
				AmountVND:  g.subtotal,
				Items:      toCalcItems(g.items),
			})
			if err != nil {
				return nil, fmt.Errorf("shipping quote for brand %s: %w", bID, err)
			}
			for i, o := range opts {
				options = append(options, domain.ShippingOptionResp{
					Carrier: o.Carrier, CarrierName: o.CarrierName, Service: o.Service,
					AmountVND: o.AmountVND, ETA: o.ETA,
				})
				if i == 0 || o.AmountVND < cheapest {
					cheapest = o.AmountVND
				}
			}
		}
		subOrders = append(subOrders, domain.CheckoutPreviewSubOrder{
			Brand:           g.brand,
			Items:           g.items,
			SubtotalVND:     g.subtotal,
			ShippingFeeVND:  cheapest,
			TotalVND:        g.subtotal + cheapest,
			ShippingOptions: options,
		})
		shippingAll += cheapest
	}

	grand := subtotalAll + shippingAll
	return &domain.CheckoutPreviewResp{
		CartEmpty:         false,
		Address:           &shipAddr,
		SubOrders:         subOrders,
		SubtotalVND:       subtotalAll,
		ShippingTotalVND:  shippingAll,
		GrandTotalVND:     grand,
		MinOrderValueVND:  domain.MinOrderValueVND,
		MeetsMinOrder:     subtotalAll >= domain.MinOrderValueVND,
		Warnings:          warnings,
		AddressIncomplete: addrIncomplete,
	}, nil
}

// variantLabel builds a human-readable label from colour/size, falling back to SKU.
func variantLabel(ci *cartdomain.CartItemView) string {
	if ci.Color != "" && ci.Size != "" {
		return ci.Color + " / " + ci.Size
	}
	if ci.Color != "" {
		return ci.Color
	}
	if ci.Size != "" {
		return ci.Size
	}
	return ci.SKU
}

func toShippingProviderAddr(a domain.ShippingAddress) provider.ShippingAddress {
	return provider.ShippingAddress{
		Recipient: a.Recipient,
		Phone:     a.Phone,
		Line1:     a.Line1,
		Ward:      a.Ward,
		District:  a.District,
		City:      a.City,
	}
}

// toCalcItems converts preview items to provider CalcItems.
// Dimension/weight pointers are left nil — the provider applies config defaults.
func toCalcItems(items []domain.CheckoutPreviewItem) []provider.CalcItem {
	out := make([]provider.CalcItem, 0, len(items))
	for _, it := range items {
		out = append(out, provider.CalcItem{
			VariantID: it.VariantID,
			ProductID: it.ProductID,
			Qty:       it.Qty,
		})
	}
	return out
}
