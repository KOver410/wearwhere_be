package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

type AddressPG struct{ db DBTX }

func NewAddressPG(db DBTX) *AddressPG { return &AddressPG{db: db} }

const addrCols = `id, brand_id, label, address_line, ward, district, city,
                  city_code, district_code, ward_code,
                  country, postal_code, phone, latitude, longitude,
                  is_primary, is_public, created_at, updated_at, deleted_at`

func scanAddress(row pgx.Row) (*domain.BrandAddress, error) {
	var a domain.BrandAddress
	err := row.Scan(
		&a.ID, &a.BrandID, &a.Label, &a.AddressLine, &a.Ward, &a.District, &a.City,
		&a.CityCode, &a.DistrictCode, &a.WardCode,
		&a.Country, &a.PostalCode, &a.Phone, &a.Latitude, &a.Longitude,
		&a.IsPrimary, &a.IsPublic, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *AddressPG) List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
	q := `SELECT ` + addrCols + ` FROM brand_addresses
          WHERE brand_id=$1 AND deleted_at IS NULL`
	if !includePrivate {
		q += ` AND is_public = TRUE`
	}
	q += ` ORDER BY is_primary DESC, created_at ASC`

	rows, err := r.db.Query(ctx, q, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*domain.BrandAddress
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *AddressPG) FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error) {
	return scanAddress(r.db.QueryRow(ctx,
		`SELECT `+addrCols+` FROM brand_addresses
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID))
}

func (r *AddressPG) Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
	country := req.Country
	if country == "" {
		country = "VN"
	}
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	// Demote existing primary if new one will be primary.
	if req.IsPrimary {
		if _, err := r.db.Exec(ctx,
			`UPDATE brand_addresses SET is_primary = FALSE, updated_at = NOW()
             WHERE brand_id=$1 AND is_primary AND deleted_at IS NULL`,
			brandID); err != nil {
			return nil, err
		}
	}

	row := r.db.QueryRow(ctx,
		`INSERT INTO brand_addresses
         (brand_id, label, address_line, ward, district, city,
          city_code, district_code, ward_code,
          country, postal_code, phone, latitude, longitude, is_primary, is_public)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
         RETURNING `+addrCols,
		brandID, req.Label, req.AddressLine, req.Ward, req.District, req.City,
		nil, nil, nil,
		country, req.PostalCode, req.Phone, req.Latitude, req.Longitude,
		req.IsPrimary, isPublic)
	return scanAddress(row)
}

func (r *AddressPG) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
	// If becoming primary, demote others first.
	if req.IsPrimary != nil && *req.IsPrimary {
		if _, err := r.db.Exec(ctx,
			`UPDATE brand_addresses SET is_primary = FALSE, updated_at = NOW()
             WHERE brand_id=$1 AND id <> $2 AND is_primary AND deleted_at IS NULL`,
			brandID, id); err != nil {
			return nil, err
		}
	}
	row := r.db.QueryRow(ctx,
		`UPDATE brand_addresses SET
           label         = COALESCE($3, label),
           address_line  = COALESCE($4, address_line),
           ward          = COALESCE($5, ward),
           district      = COALESCE($6, district),
           city          = COALESCE($7, city),
           city_code     = COALESCE($8, city_code),
           district_code = COALESCE($9, district_code),
           ward_code     = COALESCE($10, ward_code),
           country       = COALESCE($11, country),
           postal_code   = COALESCE($12, postal_code),
           phone         = COALESCE($13, phone),
           latitude      = COALESCE($14, latitude),
           longitude     = COALESCE($15, longitude),
           is_primary    = COALESCE($16, is_primary),
           is_public     = COALESCE($17, is_public),
           updated_at    = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL
         RETURNING `+addrCols,
		id, brandID, req.Label, req.AddressLine, req.Ward, req.District, req.City,
		nil, nil, nil,
		req.Country, req.PostalCode, req.Phone, req.Latitude, req.Longitude,
		req.IsPrimary, req.IsPublic)
	return scanAddress(row)
}

func (r *AddressPG) PrimaryAddressCodes(ctx context.Context, brandID uuid.UUID) (string, string, error) {
	var city, district *string
	err := r.db.QueryRow(ctx,
		`SELECT city_code, district_code FROM brand_addresses
		  WHERE brand_id = $1 AND is_primary = TRUE AND deleted_at IS NULL
		  LIMIT 1`, brandID).Scan(&city, &district)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	c, d := "", ""
	if city != nil {
		c = *city
	}
	if district != nil {
		d = *district
	}
	return c, d, nil
}

func (r *AddressPG) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE brand_addresses SET deleted_at = NOW(), updated_at = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
