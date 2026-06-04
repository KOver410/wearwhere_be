package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	branddomain "github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

type brandPickupRepo interface {
	PrimaryAddress(ctx context.Context, brandID uuid.UUID) (*branddomain.BrandAddress, error)
}

type FulfillmentService struct {
	orderRepo orderrepo.OrderRepo
	subOrder  orderrepo.SubOrderRepo
	items     orderrepo.OrderItemRepo
	goship    goship.Service
	brandAddr brandPickupRepo
	defaults  weight.Defaults
}

func NewFulfillmentService(
	or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo,
	ir orderrepo.OrderItemRepo, gs goship.Service, ba brandPickupRepo, d weight.Defaults,
) *FulfillmentService {
	return &FulfillmentService{orderRepo: or, subOrder: sr, items: ir, goship: gs, brandAddr: ba, defaults: d}
}

func (s *FulfillmentService) loadOwned(ctx context.Context, brandID, subOrderID uuid.UUID) (*domain.SubOrder, error) {
	so, err := s.subOrder.GetByID(ctx, subOrderID)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return nil, domain.ErrSubOrderNotFound
		}
		return nil, err
	}
	if so.BrandID != brandID {
		return nil, domain.ErrNotBrandOwner
	}
	return so, nil
}

func (s *FulfillmentService) List(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) (*domain.BrandSubOrderListResp, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	rows, total, err := s.subOrder.ListByBrand(ctx, brandID, statuses, page, pageSize)
	if err != nil {
		return nil, err
	}
	out := make([]domain.BrandSubOrderListItem, 0, len(rows))
	// TODO(perf): N+1 — fetches the parent order + items per sub-order. For a
	// high-traffic brand dashboard, fold order_no/recipient/item_count into the
	// ListByBrand query (JOIN orders + COUNT) to make this a single round-trip.
	for _, so := range rows {
		ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
		if err != nil {
			return nil, err
		}
		its, err := s.items.ListBySubOrderID(ctx, so.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.BrandSubOrderListItem{
			SubOrderID: so.ID, OrderNo: ord.OrderNo, Status: so.Status,
			Recipient: ord.ShippingAddress.Recipient, TotalVND: so.TotalVND,
			ItemCount: len(its), TrackingNo: so.TrackingNo, CreatedAt: so.CreatedAt,
		})
	}
	totalPages := (total + pageSize - 1) / pageSize
	return &domain.BrandSubOrderListResp{Data: out, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *FulfillmentService) Detail(ctx context.Context, brandID, subOrderID uuid.UUID) (*domain.BrandSubOrderDetailResp, error) {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return nil, err
	}
	ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
	if err != nil {
		return nil, err
	}
	its, err := s.items.ListBySubOrderID(ctx, so.ID)
	if err != nil {
		return nil, err
	}
	itemResps := make([]domain.OrderItemResp, 0, len(its))
	for _, it := range its {
		itemResps = append(itemResps, domain.OrderItemResp{
			ID: it.ID, VariantID: it.VariantID, ProductID: it.ProductID,
			ProductName: it.ProductName, VariantLabel: it.VariantLabel, ImageURL: it.ImageURL,
			Qty: it.Qty, UnitPriceVND: it.UnitPriceVND, LineTotalVND: it.LineTotalVND,
		})
	}
	return &domain.BrandSubOrderDetailResp{
		SubOrderID: so.ID, OrderNo: ord.OrderNo, Status: so.Status,
		SubtotalVND: so.SubtotalVND, ShippingFeeVND: so.ShippingFeeVND, TotalVND: so.TotalVND,
		ShippingCarrier: so.ShippingCarrier, TrackingNo: so.TrackingNo, TrackingURL: so.TrackingURL,
		ShippingStatusText: so.ShippingStatusText, ShippingAddress: ord.ShippingAddress,
		Items: itemResps, CreatedAt: so.CreatedAt,
	}, nil
}

func (s *FulfillmentService) Confirm(ctx context.Context, brandID, subOrderID uuid.UUID) error {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return err
	}
	if !domain.CanConfirm(so.Status) {
		return domain.ErrInvalidTransition
	}
	return s.subOrder.UpdateConfirmed(ctx, nil, so.ID)
}

func (s *FulfillmentService) Ship(ctx context.Context, brandID, subOrderID uuid.UUID, carrierOverride string) error {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return err
	}
	ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
	if err != nil {
		return err
	}
	if !domain.CanShip(so.Status, ord.Status) {
		return domain.ErrInvalidTransition
	}
	to := ord.ShippingAddress
	if to.CityCode == nil || to.DistrictCode == nil {
		return domain.ErrAddressIncomplete
	}
	from, err := s.brandAddr.PrimaryAddress(ctx, brandID)
	if err != nil || from == nil || from.CityCode == nil || from.DistrictCode == nil {
		return fmt.Errorf("%w: brand pickup address incomplete", domain.ErrShipmentCreateFailed)
	}

	its, err := s.items.ListBySubOrderID(ctx, so.ID)
	if err != nil {
		return err
	}
	wItems := make([]weight.Item, 0, len(its))
	for _, it := range its {
		wItems = append(wItems, weight.Item{Qty: it.Qty})
	}
	parcel := weight.Aggregate(wItems, s.defaults)

	carrier := derefStr(so.ShippingCarrier)
	if carrierOverride != "" {
		carrier = carrierOverride
	}
	var cod int64
	if ord.PaymentMethod == domain.PaymentMethodCOD {
		cod = so.SubtotalVND + so.ShippingFeeVND
	}

	rates, err := s.goship.Rates(ctx, goship.RateReq{
		From:   goship.Address{CityCode: *from.CityCode, DistrictCode: *from.DistrictCode},
		To:     goship.Address{CityCode: *to.CityCode, DistrictCode: *to.DistrictCode},
		Parcel: goship.Parcel{WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM, CODVND: cod, AmountVND: so.SubtotalVND},
	})
	if err != nil {
		return fmt.Errorf("%w: re-quote: %v", domain.ErrShipmentCreateFailed, err)
	}
	var rate *goship.Rate
	for i := range rates {
		if rates[i].Carrier == carrier {
			rate = &rates[i]
			break
		}
	}
	if rate == nil {
		return domain.ErrCarrierUnavailable
	}

	resp, err := s.goship.CreateShipment(ctx, goship.ShipmentReq{
		RateID:   rate.ID,
		From:     shipAddrFromBrand(from),
		To:       shipAddrFromSnapshot(to),
		Parcel:   goship.Parcel{WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM, CODVND: cod, AmountVND: so.SubtotalVND},
		OrderRef: so.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrShipmentCreateFailed, err)
	}
	return s.subOrder.UpdateShipped(ctx, nil, so.ID, resp.TrackingCode, resp.GoshipCode, rate.Carrier, resp.FeeVND, resp.LabelURL)
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func shipAddrFromBrand(a *branddomain.BrandAddress) goship.ShipmentAddress {
	return goship.ShipmentAddress{
		Name: a.Label, Phone: derefStr(a.Phone), Street: a.AddressLine,
		WardCode: derefStr(a.WardCode), DistrictCode: derefStr(a.DistrictCode), CityCode: derefStr(a.CityCode),
	}
}

func shipAddrFromSnapshot(a domain.ShippingAddress) goship.ShipmentAddress {
	return goship.ShipmentAddress{
		Name: a.Recipient, Phone: a.Phone, Street: a.Line1,
		WardCode: derefStr(a.WardCode), DistrictCode: derefStr(a.DistrictCode), CityCode: derefStr(a.CityCode),
	}
}
