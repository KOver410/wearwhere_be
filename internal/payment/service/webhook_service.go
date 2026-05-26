// internal/payment/service/webhook_service.go
package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

// WebhookService handles incoming PayOS webhook events.
type WebhookService struct {
	pool        *pgxpool.Pool
	paymentRepo paymentrepo.PaymentRepo
	orderRepo   orderrepo.OrderRepo
	subOrder    orderrepo.SubOrderRepo
	items       orderrepo.OrderItemRepo
	variant     productrepo.VariantRepo
	payosClient payos.Client
}

// NewWebhookService constructs a WebhookService with all required dependencies.
func NewWebhookService(
	pool *pgxpool.Pool,
	pr paymentrepo.PaymentRepo, or orderrepo.OrderRepo,
	sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	vr productrepo.VariantRepo, pc payos.Client,
) *WebhookService {
	return &WebhookService{
		pool: pool, paymentRepo: pr, orderRepo: or, subOrder: sr,
		items: ir, variant: vr, payosClient: pc,
	}
}

// HandlePayosWebhook is idempotent. Signature must already be verified by the caller.
func (s *WebhookService) HandlePayosWebhook(ctx context.Context, p payos.WebhookPayload) error {
	raw, _ := json.Marshal(p)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	payment, err := s.paymentRepo.GetByPayosOrderCodeForUpdate(ctx, tx, p.Data.OrderCode)
	if err != nil {
		if errors.Is(err, paymentdomain.ErrPaymentNotFound) {
			return nil // unknown order code — ignore safely
		}
		return err
	}
	if payment.Status != orderdomain.PaymentStatusPending {
		return nil // idempotent — already processed
	}

	items, err := s.items.ListByOrderID(ctx, payment.OrderID)
	if err != nil {
		return err
	}

	if p.Success && p.Code == "00" {
		if err := s.paymentRepo.UpdateOnPaid(ctx, tx, payment.ID, raw); err != nil {
			return err
		}
		if err := s.orderRepo.UpdateStatusOnPaid(ctx, tx, payment.OrderID); err != nil {
			return err
		}
		for _, it := range items {
			if err := s.variant.Commit(ctx, tx, it.VariantID, it.Qty); err != nil {
				return err
			}
		}
	} else {
		if err := s.paymentRepo.UpdateOnFailed(ctx, tx, payment.ID, p.Desc, raw); err != nil {
			return err
		}
		if err := s.orderRepo.UpdateStatusOnCancel(ctx, tx, payment.OrderID,
			"payos_payment_failed", orderdomain.PaymentStatusFailed); err != nil {
			return err
		}
		if err := s.subOrder.CancelAllByOrderID(ctx, tx, payment.OrderID); err != nil {
			return err
		}
		for _, it := range items {
			if err := s.variant.Release(ctx, tx, it.VariantID, it.Qty); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}
