// internal/order/service/order_service.go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

// OrderService implements the PlaceOrder atomic transaction flow.
type OrderService struct {
	pool         *pgxpool.Pool
	orderRepo    orderrepo.OrderRepo
	subOrderRepo orderrepo.SubOrderRepo
	itemRepo     orderrepo.OrderItemRepo
	paymentRepo  paymentrepo.PaymentRepo
	variantRepo  productrepo.VariantRepo
	addrRepo     customeraddrrepo.AddressRepo
	userRepo     authrepo.UserRepo
	shipping     provider.ShippingProvider
	payosClient  payos.Client
	cfg          Config
}

// Config holds service-level tunables.
type Config struct {
	ReservationTimeout time.Duration // default 30 min
	PayosReturnURL     string
	PayosCancelURL     string
}

func NewOrderService(
	pool *pgxpool.Pool,
	or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
	ar customeraddrrepo.AddressRepo, ur authrepo.UserRepo,
	ship provider.ShippingProvider, pc payos.Client, cfg Config,
) *OrderService {
	if cfg.ReservationTimeout == 0 {
		cfg.ReservationTimeout = 30 * time.Minute
	}
	return &OrderService{
		pool:      pool,
		orderRepo: or, subOrderRepo: sr, itemRepo: ir, paymentRepo: pr,
		variantRepo: vr, addrRepo: ar, userRepo: ur,
		shipping: ship, payosClient: pc, cfg: cfg,
	}
}

// cartSnapshotRow is the denormalised snapshot read inside the tx.
type cartSnapshotRow struct {
	VariantID    uuid.UUID
	Qty          int
	PriceVND     int64 // price_snapshot cast to int64 (VND, no sub-unit)
	StockQty     int
	ReservedQty  int
	IsActive     bool
	VariantDel   *time.Time
	ProductID    uuid.UUID
	VariantLabel string // "color/size" — trimmed by caller
	ProductName  string
	BrandID      uuid.UUID
	ProductDel   *time.Time
	BrandSlug    string
	BrandName    string
	ImageURL     *string
	WeightG      *int
	LengthCM     *int
	WidthCM      *int
	HeightCM     *int
}

// payosCodeSeq is an atomic counter used by nextPayosCode.
var payosCodeSeq atomic.Int64

// nextPayosCode returns a monotonic int64 suitable as a PayOS order code.
func nextPayosCode() int64 {
	return time.Now().UnixMilli()*1000 + (payosCodeSeq.Add(1) % 1000)
}

// truncate25 keeps strings ≤ 25 bytes (PayOS description limit).
func truncate25(s string) string {
	if len(s) <= 25 {
		return s
	}
	return s[:25]
}

// stringOrEmpty dereferences a *string, returning "" for nil.
func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string { return &s }

// PlaceOrder executes the 14-step atomic placement flow.
// Returns an OrderResp, a PaymentResp, and any error.
func (s *OrderService) PlaceOrder(
	ctx context.Context,
	userID uuid.UUID,
	req domain.PlaceOrderReq,
) (*domain.OrderResp, *domain.PaymentResp, error) {
	// Step 1: validate payment method.
	if !req.PaymentMethod.Valid() {
		return nil, nil, domain.ErrInvalidPaymentMethod
	}

	// Step 2: pre-tx — load address (scoped by userID) + snapshot shipping address.
	addr, err := s.addrRepo.FindByID(ctx, req.AddressID, userID)
	if err != nil {
		return nil, nil, domain.ErrAddressNotFound
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
	if addr.CityCode == nil || addr.DistrictCode == nil || addr.WardCode == nil {
		return nil, nil, domain.ErrAddressIncomplete
	}

	// Step 3: pre-tx — load user for PayOS buyer info.
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	// Step 4: BEGIN transaction.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 5: snapshot cart with FOR UPDATE OF v (locks variant rows).
	rows, err := tx.Query(ctx,
		`SELECT ci.variant_id, ci.qty, ci.price_snapshot::bigint,
		        v.stock_qty, v.reserved_qty, v.is_active, v.deleted_at,
		        v.product_id,
		        COALESCE(v.color, '') || '/' || COALESCE(v.size, ''),
		        p.name, p.brand_id, p.deleted_at,
		        b.slug, b.name,
		        (SELECT url FROM product_images
		           WHERE product_id = p.id AND is_primary = TRUE
		           ORDER BY sort_order ASC LIMIT 1) AS image_url,
		        v.weight_g, v.length_cm, v.width_cm, v.height_cm
		   FROM cart_items ci
		   JOIN product_variants v ON v.id = ci.variant_id
		   JOIN products p ON p.id = v.product_id
		   JOIN brands b ON b.id = p.brand_id
		  WHERE ci.user_id = $1
		  FOR UPDATE OF v`,
		userID)
	if err != nil {
		return nil, nil, err
	}

	var cart []cartSnapshotRow
	for rows.Next() {
		var r cartSnapshotRow
		if err := rows.Scan(
			&r.VariantID, &r.Qty, &r.PriceVND,
			&r.StockQty, &r.ReservedQty, &r.IsActive, &r.VariantDel,
			&r.ProductID, &r.VariantLabel,
			&r.ProductName, &r.BrandID, &r.ProductDel,
			&r.BrandSlug, &r.BrandName, &r.ImageURL,
			&r.WeightG, &r.LengthCM, &r.WidthCM, &r.HeightCM,
		); err != nil {
			rows.Close()
			return nil, nil, err
		}
		cart = append(cart, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(cart) == 0 {
		return nil, nil, domain.ErrCartEmpty
	}

	// Step 6: per-item validation.
	for _, r := range cart {
		if !r.IsActive || r.VariantDel != nil || r.ProductDel != nil {
			return nil, nil, fmt.Errorf("%w: variant=%s", domain.ErrVariantUnavailable, r.VariantID)
		}
		if r.StockQty-r.ReservedQty < r.Qty {
			return nil, nil, fmt.Errorf("%w: variant=%s requested=%d available=%d",
				domain.ErrInsufficientStock, r.VariantID, r.Qty, r.StockQty-r.ReservedQty)
		}
	}

	// Step 7: group by brand + compute subtotals + shipping.
	type brandGroup struct {
		brandID   uuid.UUID
		brandSlug string
		brandName string
		rows      []cartSnapshotRow
		subtotal  int64
		shipping  int64
		carrier   string
	}
	groups := map[uuid.UUID]*brandGroup{}
	brandOrder := []uuid.UUID{}
	var subtotalAll int64
	for _, r := range cart {
		g, ok := groups[r.BrandID]
		if !ok {
			g = &brandGroup{brandID: r.BrandID, brandSlug: r.BrandSlug, brandName: r.BrandName}
			groups[r.BrandID] = g
			brandOrder = append(brandOrder, r.BrandID)
		}
		line := int64(r.Qty) * r.PriceVND
		g.rows = append(g.rows, r)
		g.subtotal += line
		subtotalAll += line
	}
	selByBrand := map[uuid.UUID]string{}
	for _, sel := range req.ShippingSelections {
		selByBrand[sel.BrandID] = sel.Carrier
	}
	var shippingAll int64
	for _, bID := range brandOrder {
		g := groups[bID]
		chosen, ok := selByBrand[g.brandID]
		if !ok {
			return nil, nil, domain.ErrCarrierNotSelected
		}
		items := make([]provider.CalcItem, 0, len(g.rows))
		for _, r := range g.rows {
			items = append(items, provider.CalcItem{
				VariantID: r.VariantID, ProductID: r.ProductID, Qty: r.Qty,
				WeightG: r.WeightG, LengthCM: r.LengthCM, WidthCM: r.WidthCM, HeightCM: r.HeightCM,
			})
		}
		var codVND int64
		if req.PaymentMethod == domain.PaymentMethodCOD {
			codVND = g.subtotal
		}
		opts, err := s.shipping.Quote(ctx, provider.CalcReq{
			BrandID:    g.brandID,
			ToAddress:  provider.ShippingAddress{Recipient: shipAddr.Recipient, Phone: shipAddr.Phone, Line1: shipAddr.Line1, Ward: shipAddr.Ward, District: shipAddr.District, City: shipAddr.City},
			ToCityCode: shipAddr.CityCode,
			ToDistrict: shipAddr.DistrictCode,
			CODVND:     codVND,
			AmountVND:  g.subtotal,
			Items:      items,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: brand %s: %v", domain.ErrShippingUnavailable, g.brandID, err)
		}
		var matched *shippingdomain.ShippingOption
		for i := range opts {
			if opts[i].Carrier == chosen {
				matched = &opts[i]
				break
			}
		}
		if matched == nil {
			return nil, nil, domain.ErrCarrierUnavailable
		}
		g.shipping = matched.AmountVND
		g.carrier = matched.Carrier
		shippingAll += matched.AmountVND
	}
	grandTotal := subtotalAll + shippingAll

	// Step 8: min-order rule (on subtotal per spec §9).
	if subtotalAll < domain.MinOrderValueVND {
		return nil, nil, domain.ErrMinOrderValue
	}

	// Step 9: create order row with retry on order_no conflict (up to 3 attempts).
	now := time.Now()
	initialStatus := domain.OrderStatusPendingPayment
	initialPayStatus := domain.PaymentStatusPending
	if req.PaymentMethod == domain.PaymentMethodCOD {
		initialStatus = domain.OrderStatusProcessing
	}

	order := &domain.Order{
		UserID:           userID,
		SubtotalVND:      subtotalAll,
		ShippingTotalVND: shippingAll,
		GrandTotalVND:    grandTotal,
		PaymentMethod:    req.PaymentMethod,
		PaymentStatus:    initialPayStatus,
		Status:           initialStatus,
		ShippingAddress:  shipAddr,
		Notes:            req.Notes,
	}
	for attempt := 0; attempt < 3; attempt++ {
		order.OrderNo = domain.GenerateOrderNo(now)
		err := s.orderRepo.Create(ctx, tx, order)
		if err == nil {
			break
		}
		if !errors.Is(err, orderrepo.ErrOrderNoConflict) {
			return nil, nil, err
		}
		if attempt == 2 {
			return nil, nil, err
		}
	}

	// Step 10 + 11: insert sub_orders + order_items + reserve stock.
	for _, bID := range brandOrder {
		g := groups[bID]
		so := &domain.SubOrder{
			OrderID:          order.ID,
			BrandID:          g.brandID,
			SubtotalVND:      g.subtotal,
			ShippingFeeVND:   g.shipping,
			TotalVND:         g.subtotal + g.shipping,
			Status:           domain.SubOrderStatusPending,
			ShippingCarrier:  strPtr(g.carrier),
			ShippingProvider: strPtr("goship"),
		}
		if err := s.subOrderRepo.Create(ctx, tx, so); err != nil {
			return nil, nil, err
		}
		so.BrandSlug = g.brandSlug
		so.BrandName = g.brandName

		for _, r := range g.rows {
			label := strings.Trim(r.VariantLabel, "/")
			if label == "" {
				label = "default"
			}
			it := &domain.OrderItem{
				SubOrderID:   so.ID,
				VariantID:    r.VariantID,
				ProductID:    r.ProductID,
				ProductName:  r.ProductName,
				VariantLabel: label,
				ImageURL:     r.ImageURL,
				Qty:          r.Qty,
				UnitPriceVND: r.PriceVND,
				LineTotalVND: int64(r.Qty) * r.PriceVND,
			}
			if err := s.itemRepo.Create(ctx, tx, it); err != nil {
				return nil, nil, err
			}
			so.Items = append(so.Items, *it)

			// Reserve stock (second safety net on top of FOR UPDATE).
			if err := s.variantRepo.Reserve(ctx, tx, r.VariantID, r.Qty); err != nil {
				if errors.Is(err, productrepo.ErrInsufficientStock) {
					return nil, nil, fmt.Errorf("%w: variant=%s qty=%d",
						domain.ErrInsufficientStock, r.VariantID, r.Qty)
				}
				return nil, nil, err
			}
		}
		order.SubOrders = append(order.SubOrders, *so)
	}

	// Step 12: create payment row.
	var payment *paymentdomain.Payment
	expiresAt := now.Add(s.cfg.ReservationTimeout)

	if req.PaymentMethod == domain.PaymentMethodCOD {
		payment = &paymentdomain.Payment{
			OrderID:   order.ID,
			AmountVND: grandTotal,
			Method:    domain.PaymentMethodCOD,
			Status:    domain.PaymentStatusPending,
		}
		if err := s.paymentRepo.Create(ctx, tx, payment); err != nil {
			return nil, nil, err
		}
	} else {
		code := nextPayosCode()
		payment = &paymentdomain.Payment{
			OrderID:        order.ID,
			AmountVND:      grandTotal,
			Method:         domain.PaymentMethodPayos,
			Status:         domain.PaymentStatusPending,
			PayosOrderCode: &code,
			ExpiredAt:      &expiresAt,
		}
		if err := s.paymentRepo.Create(ctx, tx, payment); err != nil {
			return nil, nil, err
		}

		// Build line items for PayOS.
		var lineItems []payos.LineItem
		for _, bID := range brandOrder {
			g := groups[bID]
			for _, r := range g.rows {
				lineItems = append(lineItems, payos.LineItem{
					Name:     truncate25(r.ProductName),
					Quantity: r.Qty,
					Price:    r.PriceVND,
				})
			}
		}

		link, err := s.payosClient.CreateLink(ctx, payos.CreateLinkReq{
			OrderCode:   code,
			AmountVND:   grandTotal,
			Description: truncate25(fmt.Sprintf("DH %s", order.OrderNo)),
			Items:       lineItems,
			ReturnURL:   s.cfg.PayosReturnURL + "?orderNo=" + order.OrderNo,
			CancelURL:   s.cfg.PayosCancelURL + "?orderNo=" + order.OrderNo,
			Buyer: payos.Buyer{
				Name:  user.Name,
				Phone: stringOrEmpty(user.Phone),
				Email: stringOrEmpty(user.Email),
			},
			ExpiredAt: expiresAt.Unix(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", domain.ErrPayosLinkCreate, err)
		}

		if err := s.paymentRepo.UpdatePayosLink(ctx, tx, payment.ID,
			link.PaymentLinkID, link.CheckoutURL, link.QRCode); err != nil {
			return nil, nil, err
		}
		payment.PayosPaymentLinkID = &link.PaymentLinkID
		payment.PayosCheckoutURL = &link.CheckoutURL
		payment.PayosQRCode = &link.QRCode
	}

	// Step 13: clear cart.
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE user_id = $1`, userID); err != nil {
		return nil, nil, err
	}

	// Step 14: commit.
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return orderToResp(order), paymentToResp(payment), nil
}

// orderToResp maps a domain.Order (with SubOrders populated) to the wire DTO.
func orderToResp(o *domain.Order) *domain.OrderResp {
	resp := &domain.OrderResp{
		ID:               o.ID,
		OrderNo:          o.OrderNo,
		Status:           o.Status,
		PaymentMethod:    o.PaymentMethod,
		PaymentStatus:    o.PaymentStatus,
		SubtotalVND:      o.SubtotalVND,
		ShippingTotalVND: o.ShippingTotalVND,
		GrandTotalVND:    o.GrandTotalVND,
		ShippingAddress:  o.ShippingAddress,
		Notes:            o.Notes,
		CancelReason:     o.CancelReason,
		CreatedAt:        o.CreatedAt,
		PaidAt:           o.PaidAt,
		CancelledAt:      o.CancelledAt,
	}
	for _, so := range o.SubOrders {
		sr := domain.SubOrderResp{
			ID: so.ID,
			Brand: domain.BrandRef{
				ID:   so.BrandID,
				Slug: so.BrandSlug,
				Name: so.BrandName,
			},
			SubtotalVND:    so.SubtotalVND,
			ShippingFeeVND: so.ShippingFeeVND,
			TotalVND:       so.TotalVND,
			Status:         so.Status,
			TrackingNo:     so.TrackingNo,
		}
		for _, it := range so.Items {
			sr.Items = append(sr.Items, domain.OrderItemResp{
				ID:           it.ID,
				VariantID:    it.VariantID,
				ProductID:    it.ProductID,
				ProductName:  it.ProductName,
				VariantLabel: it.VariantLabel,
				ImageURL:     it.ImageURL,
				Qty:          it.Qty,
				UnitPriceVND: it.UnitPriceVND,
				LineTotalVND: it.LineTotalVND,
			})
		}
		resp.SubOrders = append(resp.SubOrders, sr)
	}
	return resp
}

// paymentToResp maps a domain.Payment to the wire DTO.
func paymentToResp(p *paymentdomain.Payment) *domain.PaymentResp {
	return &domain.PaymentResp{
		ID:          p.ID,
		Method:      p.Method,
		Status:      p.Status,
		AmountVND:   p.AmountVND,
		CheckoutURL: p.PayosCheckoutURL,
		QRCode:      p.PayosQRCode,
		ExpiredAt:   p.ExpiredAt,
	}
}

// Detail loads a single order with all sub-orders and items for the given user.
func (s *OrderService) Detail(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.OrderResp, error) {
	o, err := s.orderRepo.GetByOrderNo(ctx, userID, orderNo)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, err
	}
	subs, err := s.subOrderRepo.ListByOrderID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.SubOrders = make([]domain.SubOrder, 0, len(subs))
	for _, so := range subs {
		items, err := s.itemRepo.ListBySubOrderID(ctx, so.ID)
		if err != nil {
			return nil, err
		}
		copySO := *so
		for _, it := range items {
			copySO.Items = append(copySO.Items, *it)
		}
		o.SubOrders = append(o.SubOrders, copySO)
	}
	return orderToResp(o), nil
}

// List returns a paginated list of orders for the given filter (must include UserID).
func (s *OrderService) List(ctx context.Context, f orderrepo.ListFilter) (*domain.OrderListResp, error) {
	items, total, err := s.orderRepo.List(ctx, f)
	if err != nil {
		return nil, err
	}

	out := make([]domain.OrderListItem, 0, len(items))
	for _, o := range items {
		var itemCount, brandCount int
		var firstImg *string
		var firstName string
		err := s.pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(oi.qty), 0)::INT,
			        COUNT(DISTINCT so.brand_id)::INT,
			        (SELECT image_url FROM order_items oi2
			          JOIN sub_orders so2 ON so2.id = oi2.sub_order_id
			          WHERE so2.order_id = $1 ORDER BY oi2.created_at ASC LIMIT 1),
			        COALESCE((SELECT product_name FROM order_items oi3
			          JOIN sub_orders so3 ON so3.id = oi3.sub_order_id
			          WHERE so3.order_id = $1 ORDER BY oi3.created_at ASC LIMIT 1), '')
			   FROM order_items oi
			   JOIN sub_orders so ON so.id = oi.sub_order_id
			  WHERE so.order_id = $1`,
			o.ID).Scan(&itemCount, &brandCount, &firstImg, &firstName)
		if err != nil {
			return nil, err
		}

		out = append(out, domain.OrderListItem{
			ID:             o.ID,
			OrderNo:        o.OrderNo,
			Status:         o.Status,
			PaymentMethod:  o.PaymentMethod,
			PaymentStatus:  o.PaymentStatus,
			GrandTotalVND:  o.GrandTotalVND,
			ItemCount:      itemCount,
			BrandCount:     brandCount,
			FirstItemImage: firstImg,
			FirstItemName:  firstName,
			CreatedAt:      o.CreatedAt,
		})
	}

	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	totalPages := (total + f.PageSize - 1) / f.PageSize
	return &domain.OrderListResp{
		Data:       out,
		Page:       f.Page,
		PageSize:   f.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// Cancel cancels an order, releases reserved stock, and returns the updated order.
func (s *OrderService) Cancel(ctx context.Context, userID uuid.UUID, orderNo, reason string) (*domain.OrderResp, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	o, err := s.orderRepo.GetByOrderNoForUpdate(ctx, tx, userID, orderNo)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, err
	}

	// Load sub_orders to evaluate the cancel decision.
	subs, err := s.subOrderRepo.ListByOrderID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, so := range subs {
		o.SubOrders = append(o.SubOrders, *so)
	}

	decision := o.CanCustomerCancel()
	if !decision.Allowed {
		if decision.Reason == "paid_not_supported" {
			return nil, domain.ErrCancelPaidNotSupported
		}
		return nil, fmt.Errorf("%w: %s", domain.ErrCancelNotAllowed, decision.Reason)
	}

	newPayStatus := domain.PaymentStatusCancelled

	if err := s.orderRepo.UpdateStatusOnCancel(ctx, tx, o.ID, reason, newPayStatus); err != nil {
		return nil, err
	}
	if err := s.subOrderRepo.CancelAllByOrderID(ctx, tx, o.ID); err != nil {
		return nil, err
	}

	pay, perr := s.paymentRepo.GetByOrderID(ctx, o.ID)
	if perr == nil && pay.Status == domain.PaymentStatusPending {
		_ = s.paymentRepo.UpdateOnCancelled(ctx, tx, pay.ID)
	}

	items, err := s.itemRepo.ListByOrderID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if err := s.variantRepo.Release(ctx, tx, it.VariantID, it.Qty); err != nil {
			return nil, fmt.Errorf("release stock for variant %s: %w", it.VariantID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Reload via Detail for the response (read-only).
	return s.Detail(ctx, userID, orderNo)
}
