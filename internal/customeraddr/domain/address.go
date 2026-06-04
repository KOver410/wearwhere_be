// Package domain defines the customer-address aggregate.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type CustomerAddress struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Label          string
	RecipientName  string
	RecipientPhone string
	AddressLine    string
	Ward           string
	District       string
	City           string
	CityCode       *string
	DistrictCode   *string
	WardCode       *string
	Country        string
	PostalCode     *string
	Note           *string
	IsDefault      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
