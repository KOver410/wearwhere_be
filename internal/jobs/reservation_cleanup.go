package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

// ReservationCleanupJob periodically cancels PayOS orders whose payment has
// been pending for longer than timeoutMin minutes, releasing the reserved stock.
type ReservationCleanupJob struct {
	pool         *pgxpool.Pool
	orderRepo    orderrepo.OrderRepo
	subOrderRepo orderrepo.SubOrderRepo
	itemRepo     orderrepo.OrderItemRepo
	paymentRepo  paymentrepo.PaymentRepo
	variantRepo  productrepo.VariantRepo
	timeoutMin   int
}

// NewReservationCleanupJob creates a new job. timeoutMin ≤ 0 defaults to 30.
func NewReservationCleanupJob(
	pool *pgxpool.Pool,
	or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
	timeoutMin int,
) *ReservationCleanupJob {
	if timeoutMin <= 0 {
		timeoutMin = 30
	}
	return &ReservationCleanupJob{
		pool: pool, orderRepo: or, subOrderRepo: sr, itemRepo: ir,
		paymentRepo: pr, variantRepo: vr, timeoutMin: timeoutMin,
	}
}

// Run starts the cleanup loop. It blocks until ctx is cancelled.
// interval ≤ 0 defaults to 5 minutes.
func (j *ReservationCleanupJob) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	log.Printf("[reservation-cleanup] starting (timeout=%dm, interval=%s)", j.timeoutMin, interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[reservation-cleanup] stopping")
			return
		case <-t.C:
			if n, err := j.CleanupOnce(ctx); err != nil {
				log.Printf("[reservation-cleanup] error: %v", err)
			} else if n > 0 {
				log.Printf("[reservation-cleanup] released %d expired orders", n)
			}
		}
	}
}

// CleanupOnce finds all PayOS payments that have been pending longer than
// timeoutMin and expires each one atomically. Returns the number of orders
// successfully expired.
func (j *ReservationCleanupJob) CleanupOnce(ctx context.Context) (int, error) {
	rows, err := j.pool.Query(ctx,
		`SELECT p.id, p.order_id FROM payments p
		  WHERE p.method = 'payos'
		    AND p.status = 'pending'
		    AND p.created_at < NOW() - make_interval(mins => $1)
		  ORDER BY p.created_at ASC
		  LIMIT 100`,
		j.timeoutMin)
	if err != nil {
		return 0, err
	}
	type expired struct{ paymentID, orderID uuid.UUID }
	var todo []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.paymentID, &e.orderID); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, e := range todo {
		if err := j.expireOne(ctx, e.paymentID, e.orderID); err != nil {
			log.Printf("[reservation-cleanup] expireOne(%s) failed: %v", e.orderID, err)
			continue
		}
		count++
	}
	return count, nil
}

// expireOne atomically expires a single payment + order and releases reserved stock.
func (j *ReservationCleanupJob) expireOne(ctx context.Context, paymentID, orderID uuid.UUID) error {
	tx, err := j.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Re-check status inside the tx with a row-level lock to prevent races.
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM payments WHERE id=$1 FOR UPDATE`, paymentID,
	).Scan(&status); err != nil {
		return err
	}
	if status != "pending" {
		// Already processed by another path (e.g. webhook arrived concurrently).
		return nil
	}

	if err := j.paymentRepo.UpdateOnExpired(ctx, tx, paymentID); err != nil {
		return err
	}
	if err := j.orderRepo.UpdateStatusOnCancel(ctx, tx, orderID, "payos_payment_timeout", orderdomain.PaymentStatusCancelled); err != nil {
		return err
	}
	if err := j.subOrderRepo.CancelAllByOrderID(ctx, tx, orderID); err != nil {
		return err
	}

	items, err := j.itemRepo.ListByOrderID(ctx, orderID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := j.variantRepo.Release(ctx, tx, it.VariantID, it.Qty); err != nil {
			return fmt.Errorf("release variant %s qty=%d: %w", it.VariantID, it.Qty, err)
		}
	}

	return tx.Commit(ctx)
}
