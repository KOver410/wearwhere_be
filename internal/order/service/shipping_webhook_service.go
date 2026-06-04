package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

// ShippingWebhookService handles incoming Goship status webhook events.
type ShippingWebhookService struct {
	pool      *pgxpool.Pool
	subOrder  orderrepo.SubOrderRepo
	orderRepo orderrepo.OrderRepo
	items     orderrepo.OrderItemRepo
	payment   paymentrepo.PaymentRepo
	variant   productrepo.VariantRepo
}

// NewShippingWebhookService constructs a ShippingWebhookService with all required dependencies.
func NewShippingWebhookService(
	pool *pgxpool.Pool, sr orderrepo.SubOrderRepo, or orderrepo.OrderRepo,
	ir orderrepo.OrderItemRepo, pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
) *ShippingWebhookService {
	return &ShippingWebhookService{pool: pool, subOrder: sr, orderRepo: or, items: ir, payment: pr, variant: vr}
}

// HandleGoshipWebhook is idempotent. Signature must be verified by the caller.
func (s *ShippingWebhookService) HandleGoshipWebhook(ctx context.Context, p goship.WebhookPayload) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	so, err := s.subOrder.GetByTrackingNoForUpdate(ctx, tx, p.Code)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return tx.Commit(ctx) // unknown tracking code — tolerate (200)
		}
		return err
	}

	switch goship.MapStatus(p.Status, p.StatusText, p.IsReturn, p.IsLost) {
	case goship.CategoryDelivered:
		if so.Status == domain.SubOrderStatusDelivered {
			return tx.Commit(ctx) // idempotent
		}
		if err := s.subOrder.UpdateDelivered(ctx, tx, so.ID, p.StatusText, p.TrackingURL); err != nil {
			return err
		}
		ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
		if err != nil {
			return err
		}
		if ord.PaymentMethod == domain.PaymentMethodCOD {
			its, err := s.items.ListBySubOrderID(ctx, so.ID)
			if err != nil {
				return err
			}
			for _, it := range its {
				if err := s.variant.Commit(ctx, tx, it.VariantID, it.Qty); err != nil {
					return err
				}
			}
		}
		allDone, err := s.subOrder.AllDelivered(ctx, tx, so.OrderID)
		if err != nil {
			return err
		}
		if allDone {
			if err := s.orderRepo.UpdateStatusOnComplete(ctx, tx, so.OrderID); err != nil {
				return err
			}
			if ord.PaymentMethod == domain.PaymentMethodCOD {
				pay, err := s.payment.GetByOrderID(ctx, so.OrderID)
				if err == nil && pay.Status == domain.PaymentStatusPending {
					if err := s.payment.UpdateOnPaid(ctx, tx, pay.ID, []byte(`{"source":"cod_delivered"}`)); err != nil {
						return err
					}
				} else if err != nil && !errors.Is(err, paymentdomain.ErrPaymentNotFound) {
					return err
				}
			}
		}
	default: // CategoryShipped + CategoryOther — record status text / advance to shipped if applicable
		if err := s.subOrder.UpdateShippingStatus(ctx, tx, so.ID, p.StatusText, p.TrackingURL); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
