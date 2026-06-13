package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/store/domain"
)

type StorePG struct{ db DBTX }

func NewStorePG(db DBTX) *StorePG { return &StorePG{db: db} }

const storeCols = `ba.id, ba.brand_id, b.name, b.slug, b.logo_url, b.banner_url,
                   ba.label, ba.address_line, ba.ward, ba.district, ba.city,
                   ba.phone, ba.latitude, ba.longitude`

const storeFrom = `FROM brand_addresses ba
                   JOIN brands b ON b.id = ba.brand_id
                   WHERE ba.is_public = TRUE AND ba.deleted_at IS NULL
                     AND ba.latitude IS NOT NULL AND ba.longitude IS NOT NULL
                     AND b.status = 'active' AND b.deleted_at IS NULL`

func scanStore(row pgx.Row) (*domain.Store, error) {
	var s domain.Store
	err := row.Scan(
		&s.AddressID, &s.BrandID, &s.BrandName, &s.BrandSlug, &s.LogoURL, &s.BannerURL,
		&s.Label, &s.AddressLine, &s.Ward, &s.District, &s.City,
		&s.Phone, &s.Latitude, &s.Longitude,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *StorePG) collect(rows pgx.Rows) ([]*domain.Store, error) {
	defer rows.Close()
	var out []*domain.Store
	for rows.Next() {
		s, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Nearby uses a bounding-box pre-filter on the indexed lat/lng, then orders by
// the haversine expression. Cheap and index-friendly; exact ordering done in SQL.
func (r *StorePG) Nearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]*domain.Store, error) {
	q := `SELECT ` + storeCols + `,
	      6371 * 2 * ASIN(SQRT(
	        POWER(SIN(RADIANS(ba.latitude - $1)/2), 2) +
	        COS(RADIANS($1)) * COS(RADIANS(ba.latitude)) *
	        POWER(SIN(RADIANS(ba.longitude - $2)/2), 2)
	      )) AS dist_km
	      ` + storeFrom + `
	        AND ba.latitude BETWEEN $1 - ($3/111.0) AND $1 + ($3/111.0)
	        AND ba.longitude BETWEEN $2 - ($3/(111.0*COS(RADIANS($1)))) AND $2 + ($3/(111.0*COS(RADIANS($1))))
	      ORDER BY dist_km ASC
	      LIMIT $4`
	rows, err := r.db.Query(ctx, q, lat, lng, radiusKm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Store
	for rows.Next() {
		var s domain.Store
		var distKm float64
		if err := rows.Scan(
			&s.AddressID, &s.BrandID, &s.BrandName, &s.BrandSlug, &s.LogoURL, &s.BannerURL,
			&s.Label, &s.AddressLine, &s.Ward, &s.District, &s.City,
			&s.Phone, &s.Latitude, &s.Longitude, &distKm,
		); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *StorePG) SearchByArea(ctx context.Context, f AreaFilter) ([]*domain.Store, error) {
	q := `SELECT ` + storeCols + ` ` + storeFrom
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		q += cond + "$" + strconv.Itoa(len(args))
	}
	if f.CityCode != "" {
		add(" AND ba.city_code = ", f.CityCode)
	}
	if f.DistrictCode != "" {
		add(" AND ba.district_code = ", f.DistrictCode)
	}
	if f.WardCode != "" {
		add(" AND ba.ward_code = ", f.WardCode)
	}
	if f.Q != "" {
		args = append(args, "%"+strings.ToLower(f.Q)+"%")
		q += " AND (LOWER(ba.address_line) LIKE $" + strconv.Itoa(len(args)) +
			" OR LOWER(ba.district) LIKE $" + strconv.Itoa(len(args)) +
			" OR LOWER(ba.city) LIKE $" + strconv.Itoa(len(args)) + ")"
	}
	q += " ORDER BY b.name ASC"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += " LIMIT $" + strconv.Itoa(len(args))
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return r.collect(rows)
}

func (r *StorePG) Detail(ctx context.Context, addressID uuid.UUID) (*domain.Store, error) {
	q := `SELECT ` + storeCols + ` ` + storeFrom + ` AND ba.id = $1`
	s, err := scanStore(r.db.QueryRow(ctx, q, addressID))
	if err != nil {
		return nil, err
	}
	hours, err := r.hours(ctx, addressID)
	if err != nil {
		return nil, err
	}
	s.Hours = hours
	return s, nil
}

func (r *StorePG) hours(ctx context.Context, addressID uuid.UUID) ([]domain.StoreHours, error) {
	rows, err := r.db.Query(ctx,
		`SELECT weekday, to_char(open_time,'HH24:MI'), to_char(close_time,'HH24:MI')
		   FROM store_hours WHERE brand_address_id = $1
		   ORDER BY weekday, open_time`, addressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StoreHours
	for rows.Next() {
		var h domain.StoreHours
		if err := rows.Scan(&h.Weekday, &h.OpenTime, &h.CloseTime); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
