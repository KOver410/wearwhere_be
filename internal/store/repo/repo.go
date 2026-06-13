// Package repo defines persistence for store discovery (read-only over brand_addresses).
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/store/domain"
)

var ErrNotFound = errors.New("store: not found")

// DBTX is the subset of pgxpool.Pool used by this repo.
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// AreaFilter is the search-by-area criteria (any subset; q is address text).
type AreaFilter struct {
	CityCode     string
	DistrictCode string
	WardCode     string
	Q            string
	Limit        int
}

type Repo interface {
	// Nearby returns public geocoded stores within radiusKm of (lat,lng),
	// already sorted by haversine distance ascending, capped at limit.
	Nearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]*domain.Store, error)
	// SearchByArea returns stores matching the filter.
	SearchByArea(ctx context.Context, f AreaFilter) ([]*domain.Store, error)
	// Detail returns one store by brand_address id (ErrNotFound if not a public store).
	Detail(ctx context.Context, addressID uuid.UUID) (*domain.Store, error)
}
