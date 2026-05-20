package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
)

type AddressPG struct{ db DBTX }

func NewAddressPG(db DBTX) *AddressPG { return &AddressPG{db: db} }

const addrCols = `id, user_id, label, recipient_name, recipient_phone,
                  address_line, ward, district, city, country, postal_code, note,
                  is_default, created_at, updated_at, deleted_at`

func scanAddress(row pgx.Row) (*domain.CustomerAddress, error) {
	var a domain.CustomerAddress
	err := row.Scan(
		&a.ID, &a.UserID, &a.Label, &a.RecipientName, &a.RecipientPhone,
		&a.AddressLine, &a.Ward, &a.District, &a.City, &a.Country, &a.PostalCode, &a.Note,
		&a.IsDefault, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *AddressPG) List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+addrCols+` FROM customer_addresses
         WHERE user_id=$1 AND deleted_at IS NULL
         ORDER BY is_default DESC, created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.CustomerAddress
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AddressPG) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error) {
	return scanAddress(r.db.QueryRow(ctx,
		`SELECT `+addrCols+` FROM customer_addresses
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, userID))
}

func (r *AddressPG) Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
	country := req.Country
	if country == "" {
		country = "VN"
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// First-address auto-default OR explicit IsDefault → unset siblings.
	var hasExisting bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM customer_addresses
         WHERE user_id=$1 AND deleted_at IS NULL)`, userID).Scan(&hasExisting); err != nil {
		return nil, err
	}
	isDefault := req.IsDefault || !hasExisting
	if isDefault && hasExisting {
		if _, err := tx.Exec(ctx,
			`UPDATE customer_addresses SET is_default=FALSE, updated_at=NOW()
             WHERE user_id=$1 AND is_default AND deleted_at IS NULL`, userID); err != nil {
			return nil, err
		}
	}

	a, err := scanAddress(tx.QueryRow(ctx,
		`INSERT INTO customer_addresses
           (user_id, label, recipient_name, recipient_phone, address_line,
            ward, district, city, country, postal_code, note, is_default)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
         RETURNING `+addrCols,
		userID, req.Label, req.RecipientName, req.RecipientPhone, req.AddressLine,
		req.Ward, req.District, req.City, country, req.PostalCode, req.Note, isDefault))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AddressPG) Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if req.IsDefault != nil && *req.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE customer_addresses SET is_default=FALSE, updated_at=NOW()
             WHERE user_id=$1 AND id<>$2 AND is_default AND deleted_at IS NULL`,
			userID, id); err != nil {
			return nil, err
		}
	}

	a, err := scanAddress(tx.QueryRow(ctx,
		`UPDATE customer_addresses SET
            label           = COALESCE($3, label),
            recipient_name  = COALESCE($4, recipient_name),
            recipient_phone = COALESCE($5, recipient_phone),
            address_line    = COALESCE($6, address_line),
            ward            = COALESCE($7, ward),
            district        = COALESCE($8, district),
            city            = COALESCE($9, city),
            country         = COALESCE($10, country),
            postal_code     = COALESCE($11, postal_code),
            note            = COALESCE($12, note),
            is_default      = COALESCE($13, is_default),
            updated_at      = NOW()
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL
         RETURNING `+addrCols,
		id, userID, req.Label, req.RecipientName, req.RecipientPhone, req.AddressLine,
		req.Ward, req.District, req.City, req.Country, req.PostalCode, req.Note, req.IsDefault))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// SoftDelete uses SELECT ... FOR UPDATE to capture is_default BEFORE zeroing it,
// then promotes the oldest live address to default if necessary.
func (r *AddressPG) SoftDelete(ctx context.Context, id, userID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var wasDefault bool
	err = tx.QueryRow(ctx,
		`SELECT is_default FROM customer_addresses
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL
         FOR UPDATE`, id, userID).Scan(&wasDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customer_addresses
            SET deleted_at=NOW(), is_default=FALSE, updated_at=NOW()
          WHERE id=$1 AND user_id=$2`, id, userID); err != nil {
		return err
	}
	if wasDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE customer_addresses SET is_default=TRUE, updated_at=NOW()
             WHERE id = (
               SELECT id FROM customer_addresses
               WHERE user_id=$1 AND deleted_at IS NULL
               ORDER BY created_at ASC LIMIT 1
             )`, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
