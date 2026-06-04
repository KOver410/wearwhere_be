// internal/order/repo/repo.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

var (
	ErrNotFound        = errors.New("order repo: not found")
	ErrOrderNoConflict = errors.New("order repo: order_no conflict")
)

// DBTX abstracts over *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ListFilter struct {
	UserID   uuid.UUID
	Statuses []domain.OrderStatus // empty = no filter
	FromTime *string              // RFC3339 string from query
	ToTime   *string
	Page     int
	PageSize int
}

type OrderRepo interface {
	Create(ctx context.Context, db DBTX, o *domain.Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	GetByOrderNo(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.Order, error)
	GetByOrderNoForUpdate(ctx context.Context, db DBTX, userID uuid.UUID, orderNo string) (*domain.Order, error)
	List(ctx context.Context, f ListFilter) (items []*domain.Order, total int, err error)
	UpdateStatusOnPaid(ctx context.Context, db DBTX, orderID uuid.UUID) error
	UpdateStatusOnCancel(ctx context.Context, db DBTX, orderID uuid.UUID, reason string, paymentStatus domain.PaymentStatus) error
	UpdateStatusOnComplete(ctx context.Context, db DBTX, orderID uuid.UUID) error
}

type SubOrderRepo interface {
	Create(ctx context.Context, db DBTX, so *domain.SubOrder) error
	ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.SubOrder, error)
	CancelAllByOrderID(ctx context.Context, db DBTX, orderID uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SubOrder, error)
	GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (*domain.SubOrder, error)
	GetByTrackingNoForUpdate(ctx context.Context, db DBTX, trackingNo string) (*domain.SubOrder, error)
	ListByBrand(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) (items []*domain.SubOrder, total int, err error)
	UpdateConfirmed(ctx context.Context, db DBTX, id uuid.UUID) error
	UpdateShipped(ctx context.Context, db DBTX, id uuid.UUID, trackingNo, goshipCode, carrier string, costVND int64, trackingURL string) error
	UpdateDelivered(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error
	UpdateShippingStatus(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error
	AllDelivered(ctx context.Context, db DBTX, orderID uuid.UUID) (bool, error)
}

type OrderItemRepo interface {
	Create(ctx context.Context, db DBTX, item *domain.OrderItem) error
	ListBySubOrderID(ctx context.Context, subOrderID uuid.UUID) ([]*domain.OrderItem, error)
	ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderItem, error) // for cleanup release
}
