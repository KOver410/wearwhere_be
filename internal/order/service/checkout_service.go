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
		Recipient: addr.RecipientName,
		Phone:     addr.RecipientPhone,
		Line1:     addr.AddressLine,
		Ward:      addr.Ward,
		District:  addr.District,
		City:      addr.City,
	}

	items, err := s.cartRepo.ListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &domain.CheckoutPreviewResp{
			CartEmpty:        true,
			Address:          &shipAddr,
			SubOrders:        []domain.CheckoutPreviewSubOrder{},
			MinOrderValueVND: domain.MinOrderValueVND,
			MeetsMinOrder:    false,
			Warnings:         []string{},
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
		quote, err := s.shipping.Calculate(ctx, provider.CalcReq{
			BrandID:   bID,
			ToAddress: toShippingProviderAddr(shipAddr),
		})
		if err != nil {
			return nil, fmt.Errorf("shipping calc for brand %s: %w", bID, err)
		}
		shippingAll += quote.AmountVND
		subOrders = append(subOrders, domain.CheckoutPreviewSubOrder{
			Brand:          g.brand,
			Items:          g.items,
			SubtotalVND:    g.subtotal,
			ShippingFeeVND: quote.AmountVND,
			TotalVND:       g.subtotal + quote.AmountVND,
		})
	}

	grand := subtotalAll + shippingAll
	return &domain.CheckoutPreviewResp{
		CartEmpty:        false,
		Address:          &shipAddr,
		SubOrders:        subOrders,
		SubtotalVND:      subtotalAll,
		ShippingTotalVND: shippingAll,
		GrandTotalVND:    grand,
		MinOrderValueVND: domain.MinOrderValueVND,
		MeetsMinOrder:    subtotalAll >= domain.MinOrderValueVND,
		Warnings:         warnings,
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
